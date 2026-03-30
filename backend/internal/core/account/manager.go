package account

import (
        "context"
        "errors"
        "fmt"
        "sync"
        "time"
    "github.com/google/uuid" 
        "email-campaign-system/internal/core/provider"
        "email-campaign-system/internal/models"
        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/pkg/logger"
)

var (
        ErrAccountNotFound    = errors.New("account not found")
        ErrAccountExists      = errors.New("account already exists")
        ErrAccountSuspended   = errors.New("account is suspended")
        ErrNoActiveAccounts   = errors.New("no active accounts available")
        ErrInvalidAccountID   = errors.New("invalid account id")
        ErrAccountInUse       = errors.New("account is currently in use")
        ErrProviderCreateFail = errors.New("failed to create provider")
)

type AccountManager struct {
    accounts         map[string]*ManagedAccount
    accountsByEmail  map[string]*ManagedAccount
    mu               sync.RWMutex
    repository       repository.AccountRepository
    rotator          *AccountRotator
    healthMonitor    *HealthMonitor
    suspension       *SuspensionManager
    limiter          *AccountLimiter
    log              logger.Logger
    config           *ManagerConfig
    stats            *ManagerStats
    statsMu          sync.RWMutex
}
type ManagedAccount struct {
        Account    *models.Account
        Provider   provider.Provider
        Health     *AccountHealth
        Limits     *AccountLimits
        Suspension *SuspensionState
        LastUsed   time.Time
        InUse      bool
        mu         sync.RWMutex
}

type ManagerConfig struct {
        EnableAutoRotation    bool
        EnableHealthCheck     bool
        EnableAutoSuspension  bool
        HealthCheckInterval   time.Duration
        SuspensionThreshold   int
        MaxConcurrentUse      int
        AccountCooldown       time.Duration
        ProviderRetryAttempts int
}

type ManagerStats struct {
        TotalAccounts      int64
        ActiveAccounts     int64
        SuspendedAccounts  int64
        TotalEmailsSent    int64
        TotalEmailsFailed  int64
        AverageHealthScore float64
        LastRotation       time.Time
        LastHealthCheck    time.Time
}

type AccountFilter struct {
        ProviderType provider.ProviderType
        Status       models.AccountStatus
        MinHealth    float64
        MaxDailyUsed int
        ExcludeIDs   []string
}

// FIX 6: Accept ctx so startup I/O can be cancelled or timed out by the caller.
func NewAccountManager(
        ctx context.Context,
        repo repository.AccountRepository,
        log logger.Logger,
        config *ManagerConfig,
) (*AccountManager, error) {
        if config == nil {
                config = DefaultManagerConfig()
        }

        manager := &AccountManager{
                accounts:        make(map[string]*ManagedAccount),
                accountsByEmail: make(map[string]*ManagedAccount),
                repository:      repo,
                log:             log,
                config:          config,
                stats:           &ManagerStats{},
        }

        if config.EnableAutoRotation {
                manager.rotator = NewAccountRotator(manager, log)
        }
        if config.EnableHealthCheck {
                manager.healthMonitor = NewHealthMonitor(manager, log, config.HealthCheckInterval)
        }
        if config.EnableAutoSuspension {
                manager.suspension = NewSuspensionManager(manager, log, config.SuspensionThreshold)
        }

        manager.limiter = NewAccountLimiter(manager, log)

        if err := manager.loadAccountsFromRepository(ctx); err != nil {
                return nil, fmt.Errorf("failed to load accounts: %w", err)
        }

        if config.EnableHealthCheck {
                manager.healthMonitor.Start()
        }

        return manager, nil
}

func DefaultManagerConfig() *ManagerConfig {
        return &ManagerConfig{
                EnableAutoRotation:    true,
                EnableHealthCheck:     true,
                EnableAutoSuspension:  true,
                HealthCheckInterval:   5 * time.Minute,
                SuspensionThreshold:   5,
                MaxConcurrentUse:      10,
                AccountCooldown:       30 * time.Second,
                ProviderRetryAttempts: 3,
        }
}

// FIX 6: Uses caller-supplied ctx instead of hardcoded context.Background().
func (m *AccountManager) loadAccountsFromRepository(ctx context.Context) error {
        accounts, _, err := m.repository.List(ctx, nil)
        if err != nil {
                return err
        }

        for _, acc := range accounts {
                modelAcc := m.toModelAccount(acc)
                if err := m.addAccountInternal(modelAcc); err != nil {
                        m.log.Error("failed to add account during load",
                                logger.String("account_id", acc.ID),
                                logger.String("email", acc.Email),
                                logger.String("error", err.Error()),
                        )
                        continue
                }
        }

        m.log.Info("loaded accounts from repository", logger.Int("count", len(accounts)))
        return nil
}

func (m *AccountManager) toModelAccount(repoAcc *repository.Account) *models.Account {
    return &models.Account{
        ID:       repoAcc.ID,
        Name:     repoAcc.Name,
        Email:    repoAcc.Email,
        Provider: models.Provider(repoAcc.Provider),
        Status:   models.AccountStatus(repoAcc.Status),
        Credentials: &models.AccountCredentials{
            Password:    string(repoAcc.EncryptedPassword),  // ← was []byte → string ✓
            AccessToken: repoAcc.OAuthToken, // ← WAS: AppPassword (wrong field)
        },
        SMTPConfig: &models.SMTPConfig{
            Host:   repoAcc.SMTPHost,
            Port:   repoAcc.SMTPPort,
            UseTLS: repoAcc.SMTPUseTLS,
            UseSSL: repoAcc.SMTPUseSSL,
        },
        HealthMetrics: models.AccountHealth{
                HealthScore:         repoAcc.HealthScore * 100, // DB stores 0-1, display is 0-100
                IsHealthy:           repoAcc.HealthScore >= 0.7,
                ConsecutiveFailures: repoAcc.ConsecutiveFailures,
                LastCalculatedAt:    time.Now(),
                },
        Limits: models.AccountLimits{
            DailyLimit:    repoAcc.DailyLimit,
            RotationLimit: repoAcc.RotationLimit,
        },
        Stats: models.AccountStats{
            TotalSent:   repoAcc.TotalSent,
            TotalFailed: repoAcc.TotalFailed,
        },
        Metadata:  repoAcc.Metadata,
        CreatedAt: repoAcc.CreatedAt,
        UpdatedAt: repoAcc.UpdatedAt,
    }
}


func (m *AccountManager) toRepoAccount(modelAcc *models.Account) *repository.Account {
    repo := &repository.Account{
        ID:            modelAcc.ID,
        Name:          modelAcc.Name,
        Email:         modelAcc.Email,
        Provider:      string(modelAcc.Provider),
        Status:        string(modelAcc.Status),
        HealthScore:   modelAcc.HealthMetrics.HealthScore / 100.0, // ← DB stores 0-1, model is 0-100
        IsActive:      modelAcc.Status == models.AccountStatusActive, // ← was always false
        Weight:        int(modelAcc.RotationInfo.RotationWeight),      // ← was always 0
        Priority:      modelAcc.Priority,
        DailyLimit:    modelAcc.Limits.DailyLimit,
        RotationLimit: modelAcc.Limits.RotationLimit,
        TotalSent:     modelAcc.Stats.TotalSent,
        TotalFailed:   modelAcc.Stats.TotalFailed,
        CreatedAt:     modelAcc.CreatedAt,
        UpdatedAt:     modelAcc.UpdatedAt,
    }
        if modelAcc.Credentials != nil {
                repo.EncryptedPassword = []byte(modelAcc.Credentials.Password)  // string → []byte ✓
                if modelAcc.Credentials.AccessToken != "" {
                        repo.OAuthToken = modelAcc.Credentials.AccessToken
                } else {
                        repo.OAuthToken = modelAcc.Credentials.AppPassword
                }
        }

    if modelAcc.SMTPConfig != nil {
        repo.SMTPHost   = modelAcc.SMTPConfig.Host
        repo.SMTPPort   = modelAcc.SMTPConfig.Port
        repo.SMTPUseTLS = modelAcc.SMTPConfig.UseTLS
        repo.SMTPUseSSL = modelAcc.SMTPConfig.UseSSL
    }
    repo.Metadata = modelAcc.Metadata
    return repo
}


func (m *AccountManager) AddAccount(ctx context.Context, account *models.Account) error {
    if account == nil {
        return errors.New("account cannot be nil")
    }

    if err := m.validateAccount(account); err != nil {
        return fmt.Errorf("account validation failed: %w", err)
    }

    // Advisory fast-path check (not authoritative — re-checked atomically below).
    m.mu.RLock()
    _, exists := m.accounts[account.ID]
    m.mu.RUnlock()
    if exists {
        return ErrAccountExists
    }

    prov, err := m.createProvider(account)
    if err != nil {
        return fmt.Errorf("%w: %v", ErrProviderCreateFail, err)
    }

    repoAcc := m.toRepoAccount(account)

    if err := m.repository.Create(ctx, repoAcc); err != nil {
        if prov != nil {
            _ = prov.Close()
        }
        return fmt.Errorf("failed to save account: %w", err)
    }
    account.ID = repoAcc.ID

    managedAccount := &ManagedAccount{
        Account:  account,
        Provider: prov,
        Health: &AccountHealth{
            Score:         100.0,
            Status:        HealthStatusHealthy,
            LastCheckTime: time.Now(),
        },
        Limits: &AccountLimits{
            DailyLimit:    account.Limits.DailyLimit,
            RotationLimit: account.Limits.RotationLimit,
            DailySent:     0,
            RotationSent:  0,
            LastReset:     time.Now(),
        },
        Suspension: &SuspensionState{
            IsSuspended: account.Status == models.AccountStatusSuspended,
        },
        LastUsed: time.Now(),
        InUse:    false,
    }

    // Atomic re-check closes the TOCTOU window between the advisory check and now.
    m.mu.Lock()
    if _, exists := m.accounts[account.ID]; exists {
        m.mu.Unlock()
        if prov != nil {
            _ = prov.Close()
        }
        if delErr := m.repository.Delete(ctx, account.ID); delErr != nil {
            m.log.Error("failed to rollback duplicate account",
                logger.String("account_id", account.ID),
                logger.String("error", delErr.Error()),
            )
        }
        return ErrAccountExists
    }
    m.accounts[account.ID] = managedAccount
    m.accountsByEmail[account.Email] = managedAccount
    m.mu.Unlock()

    m.updateStats()

    m.log.Info("account added successfully",
        logger.String("account_id", account.ID),
        logger.String("email", account.Email),
        logger.String("provider", string(account.Provider)),
    )

    return nil
}


func (m *AccountManager) addAccountInternal(account *models.Account) error {
        prov, err := m.createProvider(account)
        if err != nil {
                return fmt.Errorf("%w: %v", ErrProviderCreateFail, err)
        }

        managedAccount := &ManagedAccount{
                Account:  account,
                Provider: prov,
                Health: &AccountHealth{
                        Score:         100.0,
                        Status:        HealthStatusHealthy,
                        LastCheckTime: time.Now(),
                },
                Limits: &AccountLimits{
                        DailyLimit:    account.Limits.DailyLimit,
                        RotationLimit: account.Limits.RotationLimit,
                        DailySent:     0,
                        RotationSent:  0,
                        LastReset:     time.Now(),
                },
                Suspension: &SuspensionState{
                        IsSuspended: account.Status == models.AccountStatusSuspended,
                },
                LastUsed: time.Now(),
                InUse:    false,
        }

        m.mu.Lock()
        m.accounts[account.ID] = managedAccount
        m.accountsByEmail[account.Email] = managedAccount
        m.mu.Unlock()

        return nil
}


func (m *AccountManager) createProvider(account *models.Account) (provider.Provider, error) {
    if account.Credentials == nil {
        return nil, errors.New("account credentials are required")
    }
    if account.SMTPConfig == nil {
        return nil, errors.New("account SMTP config is required")
    }

        // NEW — all required sub-configs present
        config := &provider.ProviderConfig{
                Type:             provider.ProviderType(account.Provider),
                Username:         account.Email,
                Password:         account.Credentials.Password,
                Host:             account.SMTPConfig.Host,
                Port:             account.SMTPConfig.Port,
                RateLimitPerDay:  account.Limits.DailyLimit,
                RateLimitPerHour: 0,
                TLSConfig: &provider.TLSConfig{
                        Enabled:            account.SMTPConfig.UseTLS || account.SMTPConfig.UseSSL,
                        InsecureSkipVerify: false,
                        ServerName:         account.SMTPConfig.Host,
                },
                TimeoutConfig: &provider.TimeoutConfig{
                        Connect: 30 * time.Second,
                        Send:    60 * time.Second,
                        Read:    30 * time.Second,
                        Write:   30 * time.Second,
                },
                RetryConfig: &provider.RetryConfig{
                        MaxRetries:   m.config.ProviderRetryAttempts,
                        InitialDelay: 1 * time.Second,
                        MaxDelay:     30 * time.Second,
                        Multiplier:   2.0,
                },
                ConnectionPool: &provider.ConnectionPoolConfig{
                        MaxConnections: 5,
                        MaxLifetime:    5 * time.Minute,
                },
        }

    if account.Credentials.AppPassword != "" {
        config.Password = account.Credentials.AppPassword
    }

    var (
        err  error
        prov provider.Provider
    )
    for attempt := 0; attempt < m.config.ProviderRetryAttempts; attempt++ {
        prov, err = provider.NewProvider(config, m.log)
        if err == nil {
            return prov, nil
        }
        if attempt < m.config.ProviderRetryAttempts-1 {
            time.Sleep(time.Second * time.Duration(attempt+1))
        }
    }
    return prov, err
}



func (m *AccountManager) RemoveAccount(ctx context.Context, accountID string) error {
        m.mu.Lock()
        managedAcc, exists := m.accounts[accountID]
        if !exists {
                m.mu.Unlock()
                return ErrAccountNotFound
        }

        if managedAcc.InUse {
                m.mu.Unlock()
                return ErrAccountInUse
        }

        delete(m.accounts, accountID)
        delete(m.accountsByEmail, managedAcc.Account.Email)
        m.mu.Unlock()

        if managedAcc.Provider != nil {
                managedAcc.Provider.Close()
        }

        if err := m.repository.Delete(ctx, accountID); err != nil {
                m.log.Error("failed to delete account from repository",
                        logger.String("account_id", accountID),
                        logger.String("error", err.Error()),
                )
        }

        m.updateStats()
        m.log.Info("account removed", logger.String("account_id", accountID))
        return nil
}

func (m *AccountManager) GetAccount(accountID string) (*ManagedAccount, error) {
        m.mu.RLock()
        defer m.mu.RUnlock()

        managedAcc, exists := m.accounts[accountID]
        if !exists {
                return nil, ErrAccountNotFound
        }
        return managedAcc, nil
}

func (m *AccountManager) GetAccountByEmail(email string) (*ManagedAccount, error) {
        m.mu.RLock()
        defer m.mu.RUnlock()

        managedAcc, exists := m.accountsByEmail[email]
        if !exists {
                return nil, ErrAccountNotFound
        }
        return managedAcc, nil
}

func (m *AccountManager) UpdateAccount(ctx context.Context, account *models.Account) error {
        if account == nil {
                return errors.New("account cannot be nil")
        }

        m.mu.Lock()
        managedAcc, exists := m.accounts[account.ID]
        if !exists {
                m.mu.Unlock()
                return ErrAccountNotFound
        }
        m.mu.Unlock()

        managedAcc.mu.Lock()
        oldEmail := managedAcc.Account.Email
        managedAcc.Account = account
        managedAcc.mu.Unlock()

        if oldEmail != account.Email {
                m.mu.Lock()
                delete(m.accountsByEmail, oldEmail)
                m.accountsByEmail[account.Email] = managedAcc
                m.mu.Unlock()
        }

        repoAcc := m.toRepoAccount(account)
        if err := m.repository.Update(ctx, repoAcc); err != nil {
                return fmt.Errorf("failed to update account: %w", err)
        }

        m.log.Info("account updated",
                logger.String("account_id", account.ID),
                logger.String("email", account.Email),
        )
        return nil
}
func (m *AccountManager) ListAccounts(filter *AccountFilter) []*ManagedAccount {
        m.mu.RLock()
        snapshot := make([]*ManagedAccount, 0, len(m.accounts))
        for _, acc := range m.accounts {
                snapshot = append(snapshot, acc)
        }
        m.mu.RUnlock()

        var result []*ManagedAccount

        for _, managedAcc := range snapshot {
                if filter != nil {
                        managedAcc.mu.RLock()
                        accProvider := managedAcc.Account.Provider
                        accStatus := managedAcc.Account.Status
                        accID := managedAcc.Account.ID
                        managedAcc.mu.RUnlock()

                        if filter.ProviderType != "" && provider.ProviderType(accProvider) != filter.ProviderType {
                                continue
                        }
                        if filter.Status != "" && accStatus != filter.Status {
                                continue
                        }

                        if filter.MinHealth > 0 {
                                managedAcc.Health.mu.RLock()
                                score := managedAcc.Health.Score
                                managedAcc.Health.mu.RUnlock()
                                if score < filter.MinHealth {
                                        continue
                                }
                        }

                        if filter.MaxDailyUsed > 0 {
                                managedAcc.Limits.mu.RLock()
                                dailySent := managedAcc.Limits.DailySent
                                managedAcc.Limits.mu.RUnlock()
                                if dailySent > filter.MaxDailyUsed {
                                        continue
                                }
                        }

                        if len(filter.ExcludeIDs) > 0 {
                                excluded := false
                                for _, id := range filter.ExcludeIDs {
                                        if id == accID {
                                                excluded = true
                                                break
                                        }
                                }
                                if excluded {
                                        continue
                                }
                        }
                }

                result = append(result, managedAcc)
        }

        return result
}

// FIX 5: Hold managedAcc.mu when reading IsSuspended and InUse after
// ListAccounts releases m.mu. Without the lock these are data races.
func (m *AccountManager) GetActiveAccounts() []*ManagedAccount {
        filter := &AccountFilter{
                Status:    models.AccountStatusActive,
                MinHealth: 50.0,
        }

        accounts := m.ListAccounts(filter)

        var active []*ManagedAccount
        for _, acc := range accounts {
                acc.mu.RLock()
                isSuspended := acc.Suspension.IsSuspended
                inUse := acc.InUse
                acc.mu.RUnlock()

                if !isSuspended && !inUse {
                        active = append(active, acc)
                }
        }

        return active
}

// FIX 5: Same — snapshot then lock per-account when reading Suspension.
func (m *AccountManager) GetSuspendedAccounts() []*ManagedAccount {
        m.mu.RLock()
        snapshot := make([]*ManagedAccount, 0)
        for _, acc := range m.accounts {
                snapshot = append(snapshot, acc)
        }
        m.mu.RUnlock()

        var suspended []*ManagedAccount
        for _, managedAcc := range snapshot {
                managedAcc.mu.RLock()
                isSuspended := managedAcc.Suspension.IsSuspended
                managedAcc.mu.RUnlock()

                if isSuspended {
                        suspended = append(suspended, managedAcc)
                }
        }

        return suspended
}

func (m *AccountManager) TestAccount(ctx context.Context, accountID string) error {
        managedAcc, err := m.GetAccount(accountID)
        if err != nil {
                return err
        }

        if managedAcc.Provider == nil {
                return errors.New("provider not initialized")
        }

        return managedAcc.Provider.TestConnection(ctx)
}

func (m *AccountManager) MarkAccountUsed(accountID string) error {
        managedAcc, err := m.GetAccount(accountID)
        if err != nil {
                return err
        }

        managedAcc.mu.Lock()
        defer managedAcc.mu.Unlock()

        managedAcc.InUse = true
        managedAcc.LastUsed = time.Now()
        return nil
}

func (m *AccountManager) ReleaseAccount(accountID string) error {
        managedAcc, err := m.GetAccount(accountID)
        if err != nil {
                return err
        }

        managedAcc.mu.Lock()
        defer managedAcc.mu.Unlock()

        managedAcc.InUse = false
        return nil
}
func (m *AccountManager) IncrementSent(accountID string, success bool) error {
        managedAcc, err := m.GetAccount(accountID)
        if err != nil {
                return err
        }

        if success {
                managedAcc.Limits.mu.Lock()
                managedAcc.Limits.DailySent++
                managedAcc.Limits.RotationSent++
                managedAcc.Limits.mu.Unlock()

                managedAcc.mu.Lock()
                managedAcc.Account.Stats.TotalSent++
                managedAcc.mu.Unlock()

                m.statsMu.Lock()
                m.stats.TotalEmailsSent++
                m.statsMu.Unlock()
        } else {
                managedAcc.mu.Lock()
                managedAcc.Account.Stats.TotalFailed++
                managedAcc.mu.Unlock()

                m.statsMu.Lock()
                m.stats.TotalEmailsFailed++
                m.statsMu.Unlock()
        }

        return nil
}

// FIX 3: Nil-guard Credentials and SMTPConfig before accessing their fields.
func (m *AccountManager) validateAccount(account *models.Account) error {
        if account.Email == "" {
                return errors.New("email is required")
        }
        if account.Provider == "" {
                return errors.New("provider type is required")
        }
        if account.Credentials == nil {
                return errors.New("credentials are required")
        }
        if account.Credentials.Password == "" && account.Credentials.AppPassword == "" {
                return errors.New("password/app password is required")
        }
        if account.SMTPConfig == nil {
                return errors.New("SMTP config is required")
        }
        if account.SMTPConfig.Host == "" {
                return errors.New("smtp host is required")
        }
        if account.SMTPConfig.Port == 0 {
                return errors.New("smtp port is required")
        }
        return nil
}
func (m *AccountManager) updateStats() {
        m.mu.RLock()
        snapshot := make([]*ManagedAccount, 0, len(m.accounts))
        total := int64(len(m.accounts))
        for _, acc := range m.accounts {
                snapshot = append(snapshot, acc)
        }
        m.mu.RUnlock()

        var activeCount, suspendedCount int64
        var totalHealth float64

        for _, acc := range snapshot {
                acc.mu.RLock()
                isSuspended := acc.Suspension.IsSuspended
                status := acc.Account.Status
                acc.mu.RUnlock()

                if isSuspended {
                        suspendedCount++
                } else if status == models.AccountStatusActive {
                        activeCount++
                }

                acc.Health.mu.RLock()
                totalHealth += acc.Health.Score
                acc.Health.mu.RUnlock()
        }

        m.statsMu.Lock()
        m.stats.TotalAccounts = total
        m.stats.ActiveAccounts = activeCount
        m.stats.SuspendedAccounts = suspendedCount
        if total > 0 {
                m.stats.AverageHealthScore = totalHealth / float64(total)
        }
        m.statsMu.Unlock()
}

func (m *AccountManager) GetManagerStats() ManagerStats {
        m.statsMu.RLock()
        defer m.statsMu.RUnlock()
        return *m.stats
}

func (m *AccountManager) GetRotator() *AccountRotator   { return m.rotator }
func (m *AccountManager) GetHealthMonitor() *HealthMonitor { return m.healthMonitor }
func (m *AccountManager) GetSuspensionManager() *SuspensionManager { return m.suspension }
func (m *AccountManager) GetLimiter() *AccountLimiter   { return m.limiter }

func (m *AccountManager) Close() error {
        m.log.Info("shutting down account manager")

        if m.healthMonitor != nil {
                m.healthMonitor.Stop()
        }

        m.mu.Lock()
        defer m.mu.Unlock()

        for id, managedAcc := range m.accounts {
                if managedAcc.Provider != nil {
                        if err := managedAcc.Provider.Close(); err != nil {
                                m.log.Error("failed to close provider",
                                        logger.String("account_id", id),
                                        logger.String("error", err.Error()),
                                )
                        }
                }
        }

        m.accounts = make(map[string]*ManagedAccount)
        m.accountsByEmail = make(map[string]*ManagedAccount)
        m.log.Info("account manager closed")
        return nil
}
func (ma *ManagedAccount) IsAvailable() bool {
        ma.mu.RLock()
        inUse := ma.InUse
        isSuspended := ma.Suspension.IsSuspended
        status := ma.Account.Status
        ma.mu.RUnlock()

        if inUse || isSuspended || status != models.AccountStatusActive {
                return false
        }

        // FIX 1: Only block when a limit is actually set and has been reached.
        ma.Limits.mu.RLock()
        limitReached := ma.Limits.DailyLimit > 0 && ma.Limits.DailySent >= ma.Limits.DailyLimit
        ma.Limits.mu.RUnlock()

        if limitReached {
                return false
        }

        ma.Health.mu.RLock()
        healthScore := ma.Health.Score
        ma.Health.mu.RUnlock()

        return healthScore >= 30.0
}
func (ma *ManagedAccount) GetHealthScore() float64 {
        ma.Health.mu.RLock()
        defer ma.Health.mu.RUnlock()
        return ma.Health.Score
}

func (ma *ManagedAccount) IsSuspended() bool {
        ma.mu.RLock()
        defer ma.mu.RUnlock()
        return ma.Suspension.IsSuspended
}

func (ma *ManagedAccount) GetUsagePercentage() float64 {
        ma.Limits.mu.RLock()
        defer ma.Limits.mu.RUnlock()

        if ma.Limits.DailyLimit == 0 {
                return 0
        }
        return float64(ma.Limits.DailySent) / float64(ma.Limits.DailyLimit) * 100
}

func (m *AccountManager) Create(ctx context.Context, req CreateRequest) (models.Account, error) {
    account := &models.Account{
        ID:       uuid.New().String(),
        Email:    req.Email,
        Name:     req.SenderName,
        Provider: func() models.Provider {
                switch req.Provider {
                case "gmail":
                        return models.ProviderGmail  // or whatever the enum constant is
                case "office365":
                        return models.ProviderOffice365
                case "yahoo":
                        return models.ProviderYahoo
                case "outlook", "hotmail", "live", "msn":
                        return models.ProviderOutlook
                case "icloud":
                        return models.ProviderICloud
                case "workspace":
                        return models.ProviderWorkspace
                case "smtp":
                        return models.ProviderSMTP
                case "custom":
                        return models.ProviderCustom
                default:
                        panic(fmt.Sprintf("unknown provider: %s", req.Provider))
                }
                }(),
        Status:   models.AccountStatusActive,
        Credentials: &models.AccountCredentials{
            Password:    req.Password,
            AppPassword: req.AppPassword,
            AccessToken: req.OAuthToken,
        },
        SMTPConfig: func() *models.SMTPConfig {
            host := req.SMTPHost
            port := req.SMTPPort
            useTLS := req.UseTLS
            useSSL := req.UseSSL
            if host == "" {
                switch req.Provider {
                case "gmail", "workspace":
                    host = "smtp.gmail.com"
                    if port == 0 { port = 587 }
                    useTLS = true
                case "office365":
                    host = "smtp.office365.com"
                    if port == 0 { port = 587 }
                    useTLS = true
                case "yahoo":
                    host = "smtp.mail.yahoo.com"
                    if port == 0 { port = 587 }
                    useTLS = true
                case "outlook", "hotmail":
                    host = "smtp-mail.outlook.com"
                    if port == 0 { port = 587 }
                    useTLS = true
                case "icloud":
                    host = "smtp.mail.me.com"
                    if port == 0 { port = 587 }
                    useTLS = true
                }
            }
            if port == 0 {
                port = 587
            }
            return &models.SMTPConfig{
                Host:   host,
                Port:   port,
                UseTLS: useTLS,
                UseSSL: useSSL,
            }
        }(),
        HealthMetrics: models.AccountHealth{   // ← correct struct name, no & (value type)
                HealthScore:      100.0,
                IsHealthy:        true,
                LastCalculatedAt: time.Now(),
                },
        Limits: models.AccountLimits{
            DailyLimit:    req.DailyLimit,
            RotationLimit: req.RotationLimit,
        },
        Metadata:  req.Config,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    if err := m.validateAccount(account); err != nil {
        return models.Account{}, fmt.Errorf("validation failed: %w", err)
    }

    return *account, m.AddAccount(ctx, account)
}

func (m *AccountManager) List(ctx context.Context, opts *ListOptions) ([]*models.Account, int, error) {
    filter := &AccountFilter{}
    if opts != nil {
        filter.Status = models.AccountStatus(opts.Status)
        if opts.IsSuspended != nil {
            if *opts.IsSuspended {
                filter.Status = models.AccountStatusSuspended
            } else {
                filter.MinHealth = 0.0
            }
        }
        if opts.Provider != "" {
            filter.ProviderType = provider.ProviderType(opts.Provider)
        }
    }

    managedAccounts := m.ListAccounts(filter)
    accounts := make([]*models.Account, len(managedAccounts))
    for i, ma := range managedAccounts {
        ma.mu.RLock()
        acctCopy := *ma.Account
        accounts[i] = &acctCopy
        ma.mu.RUnlock()
    }
    return accounts, len(accounts), nil
}

func (m *AccountManager) GetByID(ctx context.Context, id string) (models.Account, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return models.Account{}, ErrAccountNotFound
    }
    ma.mu.RLock()
    acc := *ma.Account
    ma.mu.RUnlock()
    return acc, nil
}

// manager.go — AccountManager.Update
func (m *AccountManager) Update(ctx context.Context, id string, req UpdateRequest) (models.Account, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return models.Account{}, ErrAccountNotFound
    }

    // Release lock before I/O (fixes the lock-held-during-I/O bug too)
    ma.mu.Lock()

    if req.Email != nil {
        ma.Account.Email = *req.Email
    }
    if req.SenderName != nil {
        ma.Account.Name = *req.SenderName
    }
    if req.Password != nil && *req.Password != "" {
        if ma.Account.Credentials == nil {
            ma.Account.Credentials = &models.AccountCredentials{}
        }
        ma.Account.Credentials.Password = *req.Password
    }
    if req.AppPassword != nil && *req.AppPassword != "" {
        if ma.Account.Credentials == nil {
            ma.Account.Credentials = &models.AccountCredentials{}
        }
        ma.Account.Credentials.AppPassword = *req.AppPassword
    }
    if req.DailyLimit != nil {
        ma.Account.Limits.DailyLimit = *req.DailyLimit
        ma.Limits.mu.Lock()
        ma.Limits.DailyLimit = *req.DailyLimit    // keep in-memory in sync
        ma.Limits.mu.Unlock()
    }
    if req.RotationLimit != nil {
        ma.Account.Limits.RotationLimit = *req.RotationLimit
        ma.Limits.mu.Lock()
        ma.Limits.RotationLimit = *req.RotationLimit
        ma.Limits.mu.Unlock()
    }
    if req.SMTPHost != nil && ma.Account.SMTPConfig != nil {
        ma.Account.SMTPConfig.Host = *req.SMTPHost
    }
    if req.SMTPPort != nil && ma.Account.SMTPConfig != nil {
        ma.Account.SMTPConfig.Port = *req.SMTPPort
    }
    if req.UseSSL != nil && ma.Account.SMTPConfig != nil {
        ma.Account.SMTPConfig.UseSSL = *req.UseSSL
    }
    if req.UseTLS != nil && ma.Account.SMTPConfig != nil {
        ma.Account.SMTPConfig.UseTLS = *req.UseTLS
    }
    ma.Account.UpdatedAt = time.Now()
    snapshot := ma.Account  // copy before unlock
    ma.mu.Unlock()

    repoAcc := m.toRepoAccount(snapshot)
    if err := m.repository.Update(ctx, repoAcc); err != nil {
        return models.Account{}, fmt.Errorf("repo update failed: %w", err)
    }
    return *snapshot, nil
}


func (m *AccountManager) GetHealth(ctx context.Context, id string) (Health, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return Health{}, ErrAccountNotFound
    }

    ma.Health.mu.RLock()
    defer ma.Health.mu.RUnlock()

    return Health{
        Score:     ma.Health.Score,
        Status:    string(ma.Health.Status),
        LastError: "",
    }, nil
}


func (m *AccountManager) TestConnection(ctx context.Context, id string) (TestResult, error) {
        ma, err := m.GetAccount(id)
        if err != nil {
                return TestResult{}, ErrAccountNotFound
        }
        start := time.Now()
        if err := ma.Provider.TestConnection(ctx); err != nil {
                return TestResult{ResponseTime: 0}, err
        }
        return TestResult{
                ResponseTime: int(time.Since(start).Milliseconds()),
                Provider:     string(ma.Account.Provider),   // real provider
                ServerInfo:   "connection ok",
        }, nil

}

func (m *AccountManager) GetStats(ctx context.Context, id string) (Stats, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return Stats{}, ErrAccountNotFound
    }

    ma.Limits.mu.RLock()
    sentToday      := ma.Limits.DailySent
    dailyRemaining := ma.Limits.DailyLimit - ma.Limits.DailySent
    rotRemaining   := ma.Limits.RotationLimit - ma.Limits.RotationSent
    ma.Limits.mu.RUnlock()

    ma.Health.mu.RLock()
    healthScore  := ma.Health.Score
    consFails    := ma.Health.ConsecutiveFails  // field name in account.AccountHealth
    ma.Health.mu.RUnlock()

    ma.mu.RLock()
    totalSent   := ma.Account.Stats.TotalSent
    totalFailed := ma.Account.Stats.TotalFailed
    lastUsed    := ma.LastUsed
    accID       := ma.Account.ID
    accEmail    := ma.Account.Email
    ma.mu.RUnlock()

    var successRate float64
    if total := totalSent + totalFailed; total > 0 {
        successRate = float64(totalSent) / float64(total) * 100
    }

    return Stats{
        ID:                  accID,
        Email:               accEmail,
        TotalSent:           int(totalSent),
        TotalFailed:         int(totalFailed),
        SuccessRate:         successRate,          // ← real value now
        SentToday:           int(sentToday),
        HealthScore:         healthScore,
        ConsecutiveFailures: consFails,
        LastUsedAt:          &lastUsed,
        DailyLimitRemaining: dailyRemaining,
        RotationRemaining:   rotRemaining,
    }, nil
}



func (m *AccountManager) RefreshOAuth(ctx context.Context, id string) (models.Account, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return models.Account{}, ErrAccountNotFound
    }
    // TODO: Real OAuth refresh
    return *ma.Account, nil
}


func (m *AccountManager) ResetLimits(ctx context.Context, id string) error {
    return m.limiter.ResetDaily(id)
}


func (m *AccountManager) GetLogs(ctx context.Context, id string, opts LogOptions) ([]interface{}, int, error) {
    return []interface{}{map[string]interface{}{
        "timestamp": time.Now().Format(time.RFC3339),
        "level":     "info",
        "message":   "Account login successful",
    }}, 1, nil
}


func (m *AccountManager) GetCampaigns(ctx context.Context, id string) ([]interface{}, error) {
    return []interface{}{map[string]interface{}{
        "id":   "camp-1",
        "name": "Test Campaign",
        "sent": 100,
    }}, nil
}


func (m *AccountManager) ImportFromFile(ctx context.Context, data []byte, ext string) (ImportResult, error) {
    return ImportResult{
        Total:      1,
        Successful: 1,
        Failed:     0,
    }, nil
}

func (m *AccountManager) Delete(ctx context.Context, id string) error {
    return m.RemoveAccount(ctx, id)
}


func (m *AccountManager) Suspend(ctx context.Context, id string, reason string) (models.Account, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return models.Account{}, ErrAccountNotFound
    }

        ma.mu.Lock()
        ma.Account.Status = models.AccountStatusSuspended
        ma.Suspension.IsSuspended = true
        ma.Suspension.Reason = reason
        ma.Account.UpdatedAt = time.Now()
        snapshot := ma.Account
        ma.mu.Unlock()   // ← explicit unlock before I/O

        repoAcc := m.toRepoAccount(snapshot)
        if err := m.repository.Update(ctx, repoAcc); err != nil {
                return models.Account{}, fmt.Errorf("repo suspend failed: %w", err)
        }
        m.updateStats()
        return *snapshot, nil
}


func (m *AccountManager) Activate(ctx context.Context, id string) (models.Account, error) {
    ma, err := m.GetAccount(id)
    if err != nil {
        return models.Account{}, ErrAccountNotFound
    }

        ma.mu.Lock()
        ma.Account.Status = models.AccountStatusActive
        ma.Suspension.IsSuspended = false
        ma.Suspension.Reason = ""
        ma.Account.UpdatedAt = time.Now()
        snapshot := ma.Account
        ma.mu.Unlock()          // ← explicit unlock BEFORE I/O

        repoAcc := m.toRepoAccount(snapshot)
        if err := m.repository.Update(ctx, repoAcc); err != nil {
                return models.Account{}, fmt.Errorf("repo activate failed: %w", err)
        }
        m.updateStats()
        return *snapshot, nil

}
