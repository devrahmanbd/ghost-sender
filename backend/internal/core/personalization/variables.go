package personalization

const (
	VarRecipientEmail         = "RECIPIENT_EMAIL"
	VarRecipientName          = "RECIPIENT_NAME"
	VarFirstName              = "FIRST_NAME"
	VarLastName               = "LAST_NAME"
	VarEmailUsername          = "EMAIL_USERNAME"
	VarEmailDomain            = "EMAIL_DOMAIN"
	VarTodayDate              = "TODAY_DATE"
	VarTodayDateLong          = "TODAY_DATE_LONG"
	VarTodayDateShort         = "TODAY_DATE_SHORT"
	VarTodayDateMedium        = "TODAY_DATE_MEDIUM"
	VarCurrentTime            = "CURRENT_TIME"
	VarCurrentTime12          = "CURRENT_TIME_12"
	VarCurrentTime24          = "CURRENT_TIME_24"
	VarCurrentYear            = "CURRENT_YEAR"
	VarCurrentMonth           = "CURRENT_MONTH"
	VarCurrentMonthShort      = "CURRENT_MONTH_SHORT"
	VarCurrentMonthNum        = "CURRENT_MONTH_NUM"
	VarCurrentDay             = "CURRENT_DAY"
	VarCurrentDayShort        = "CURRENT_DAY_SHORT"
	VarCurrentDayNum          = "CURRENT_DAY_NUM"
	VarCurrentWeek            = "CURRENT_WEEK"
	VarCurrentQuarter         = "CURRENT_QUARTER"
	VarTimeOfDay              = "TIME_OF_DAY"
	VarGreeting               = "GREETING"
	VarInvoiceNumber          = "INVOICE_NUMBER"
	VarOrderNumber            = "ORDER_NUMBER"
	VarTrackingNumber         = "TRACKING_NUMBER"
	VarReferenceNumber        = "REFERENCE_NUMBER"
	VarConfirmationNumber     = "CONFIRMATION_NUMBER"
	VarTransactionID          = "TRANSACTION_ID"
	VarPhoneNumber            = "PHONE_NUMBER"
	VarPhoneNumberUS          = "PHONE_NUMBER_US"
	VarPhoneNumberIntl        = "PHONE_NUMBER_INTL"
	VarRandomNumber           = "RANDOM_NUMBER"
	VarRandomAlpha            = "RANDOM_ALPHA"
	VarRandomAlphanumeric     = "RANDOM_ALPHANUMERIC"
	VarRandomNum4             = "RANDOM_NUM_4"
	VarRandomNum6             = "RANDOM_NUM_6"
	VarRandomNum8             = "RANDOM_NUM_8"
	VarRandomNum10            = "RANDOM_NUM_10"
	VarRandomAlpha4           = "RANDOM_ALPHA_4"
	VarRandomAlpha6           = "RANDOM_ALPHA_6"
	VarRandomAlpha8           = "RANDOM_ALPHA_8"
	VarRandomAlpha10          = "RANDOM_ALPHA_10"
	VarUUID                   = "UUID"
	VarUUIDShort              = "UUID_SHORT"
	VarTimestamp              = "TIMESTAMP"
	VarTimestampMS            = "TIMESTAMP_MS"
	VarUnsubscribeLink        = "UNSUBSCRIBE_LINK"
	VarCompanyName            = "COMPANY_NAME"
	VarCompanyAddress         = "COMPANY_ADDRESS"
	VarCompanyPhone           = "COMPANY_PHONE"
	VarCompanyEmail           = "COMPANY_EMAIL"
	VarCompanyWebsite         = "COMPANY_WEBSITE"
	VarSupportEmail           = "SUPPORT_EMAIL"
	VarSupportPhone           = "SUPPORT_PHONE"
)

type Variable struct {
	Name              string
	Description       string
	Category          string
	Type              string
	Example           string
	RequiresRecipient bool
	RequiresCampaign  bool
	RequiresAccount   bool
	Dynamic           bool
	Deprecated        bool
}

var AllVariables = []Variable{
	{
		Name:              VarRecipientEmail,
		Description:       "Full email address of the recipient",
		Category:          "Recipient",
		Type:              "string",
		Example:           "john.doe@example.com",
		RequiresRecipient: true,
	},
	{
		Name:              VarRecipientName,
		Description:       "Full name of the recipient (extracted if not provided)",
		Category:          "Recipient",
		Type:              "string",
		Example:           "John Doe",
		RequiresRecipient: true,
	},
	{
		Name:              VarFirstName,
		Description:       "First name of the recipient",
		Category:          "Recipient",
		Type:              "string",
		Example:           "John",
		RequiresRecipient: true,
	},
	{
		Name:              VarLastName,
		Description:       "Last name of the recipient",
		Category:          "Recipient",
		Type:              "string",
		Example:           "Doe",
		RequiresRecipient: true,
	},
	{
		Name:              VarEmailUsername,
		Description:       "Username part of email (before @)",
		Category:          "Recipient",
		Type:              "string",
		Example:           "john.doe",
		RequiresRecipient: true,
	},
	{
		Name:              VarEmailDomain,
		Description:       "Domain part of email (after @)",
		Category:          "Recipient",
		Type:              "string",
		Example:           "example.com",
		RequiresRecipient: true,
	},
	{
		Name:        VarTodayDate,
		Description: "Current date in YYYY-MM-DD format",
		Category:    "Date/Time",
		Type:        "date",
		Example:     "2026-02-07",
	},
	{
		Name:        VarTodayDateLong,
		Description: "Current date in long format",
		Category:    "Date/Time",
		Type:        "date",
		Example:     "February 7, 2026",
	},
	{
		Name:        VarTodayDateShort,
		Description: "Current date in short format",
		Category:    "Date/Time",
		Type:        "date",
		Example:     "02/07/2026",
	},
	{
		Name:        VarTodayDateMedium,
		Description: "Current date in medium format",
		Category:    "Date/Time",
		Type:        "date",
		Example:     "Feb 7, 2026",
	},
	{
		Name:        VarCurrentTime,
		Description: "Current time in HH:MM:SS format",
		Category:    "Date/Time",
		Type:        "time",
		Example:     "14:30:45",
	},
	{
		Name:        VarCurrentTime12,
		Description: "Current time in 12-hour format",
		Category:    "Date/Time",
		Type:        "time",
		Example:     "2:30 PM",
	},
	{
		Name:        VarCurrentTime24,
		Description: "Current time in 24-hour format",
		Category:    "Date/Time",
		Type:        "time",
		Example:     "14:30",
	},
	{
		Name:        VarCurrentYear,
		Description: "Current year",
		Category:    "Date/Time",
		Type:        "number",
		Example:     "2026",
	},
	{
		Name:        VarCurrentMonth,
		Description: "Current month full name",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "February",
	},
	{
		Name:        VarCurrentMonthShort,
		Description: "Current month short name",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "Feb",
	},
	{
		Name:        VarCurrentMonthNum,
		Description: "Current month number (01-12)",
		Category:    "Date/Time",
		Type:        "number",
		Example:     "02",
	},
	{
		Name:        VarCurrentDay,
		Description: "Current day full name",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "Saturday",
	},
	{
		Name:        VarCurrentDayShort,
		Description: "Current day short name",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "Sat",
	},
	{
		Name:        VarCurrentDayNum,
		Description: "Current day of month (01-31)",
		Category:    "Date/Time",
		Type:        "number",
		Example:     "07",
	},
	{
		Name:        VarCurrentWeek,
		Description: "Current week number of year",
		Category:    "Date/Time",
		Type:        "number",
		Example:     "06",
	},
	{
		Name:        VarCurrentQuarter,
		Description: "Current quarter (Q1-Q4)",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "Q1",
	},
	{
		Name:        VarTimeOfDay,
		Description: "Time of day (morning/afternoon/evening/night)",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "afternoon",
	},
	{
		Name:        VarGreeting,
		Description: "Time-appropriate greeting",
		Category:    "Date/Time",
		Type:        "string",
		Example:     "Good afternoon",
	},
	{
		Name:        VarInvoiceNumber,
		Description: "Random invoice number",
		Category:    "Business",
		Type:        "string",
		Example:     "INV-2026-12345",
	},
	{
		Name:        VarOrderNumber,
		Description: "Random order number",
		Category:    "Business",
		Type:        "string",
		Example:     "ORD-2026-67890",
	},
	{
		Name:        VarTrackingNumber,
		Description: "Random tracking number",
		Category:    "Business",
		Type:        "string",
		Example:     "TRK-1234567890",
	},
	{
		Name:        VarReferenceNumber,
		Description: "Random reference number",
		Category:    "Business",
		Type:        "string",
		Example:     "REF-ABCD1234",
	},
	{
		Name:        VarConfirmationNumber,
		Description: "Random confirmation number",
		Category:    "Business",
		Type:        "string",
		Example:     "CNF-XYZ98765",
	},
	{
		Name:        VarTransactionID,
		Description: "Random transaction ID",
		Category:    "Business",
		Type:        "string",
		Example:     "TXN-4F8A2B1C",
	},
	{
		Name:        VarPhoneNumber,
		Description: "Random phone number",
		Category:    "Contact",
		Type:        "string",
		Example:     "(555) 123-4567",
	},
	{
		Name:        VarPhoneNumberUS,
		Description: "Random US phone number",
		Category:    "Contact",
		Type:        "string",
		Example:     "+1 (555) 123-4567",
	},
	{
		Name:        VarPhoneNumberIntl,
		Description: "Random international phone number",
		Category:    "Contact",
		Type:        "string",
		Example:     "+44 20 1234 5678",
	},
	{
		Name:        VarRandomNumber,
		Description: "Random 6-digit number",
		Category:    "Random",
		Type:        "number",
		Example:     "123456",
		Dynamic:     true,
	},
	{
		Name:        VarRandomAlpha,
		Description: "Random 8-character alphabetic string",
		Category:    "Random",
		Type:        "string",
		Example:     "ABCDEFGH",
		Dynamic:     true,
	},
	{
		Name:        VarRandomAlphanumeric,
		Description: "Random 10-character alphanumeric string",
		Category:    "Random",
		Type:        "string",
		Example:     "A1B2C3D4E5",
		Dynamic:     true,
	},
	{
		Name:        VarRandomNum4,
		Description: "Random 4-digit number",
		Category:    "Random",
		Type:        "number",
		Example:     "1234",
		Dynamic:     true,
	},
	{
		Name:        VarRandomNum6,
		Description: "Random 6-digit number",
		Category:    "Random",
		Type:        "number",
		Example:     "123456",
		Dynamic:     true,
	},
	{
		Name:        VarRandomNum8,
		Description: "Random 8-digit number",
		Category:    "Random",
		Type:        "number",
		Example:     "12345678",
		Dynamic:     true,
	},
	{
		Name:        VarRandomNum10,
		Description: "Random 10-digit number",
		Category:    "Random",
		Type:        "number",
		Example:     "1234567890",
		Dynamic:     true,
	},
	{
		Name:        VarRandomAlpha4,
		Description: "Random 4-character alphabetic string",
		Category:    "Random",
		Type:        "string",
		Example:     "ABCD",
		Dynamic:     true,
	},
	{
		Name:        VarRandomAlpha6,
		Description: "Random 6-character alphabetic string",
		Category:    "Random",
		Type:        "string",
		Example:     "ABCDEF",
		Dynamic:     true,
	},
	{
		Name:        VarRandomAlpha8,
		Description: "Random 8-character alphabetic string",
		Category:    "Random",
		Type:        "string",
		Example:     "ABCDEFGH",
		Dynamic:     true,
	},
	{
		Name:        VarRandomAlpha10,
		Description: "Random 10-character alphabetic string",
		Category:    "Random",
		Type:        "string",
		Example:     "ABCDEFGHIJ",
		Dynamic:     true,
	},
	{
		Name:        VarUUID,
		Description: "UUID v4 identifier",
		Category:    "Random",
		Type:        "string",
		Example:     "550e8400-e29b-41d4-a716-446655440000",
		Dynamic:     true,
	},
	{
		Name:        VarUUIDShort,
		Description: "Short UUID (first 8 characters)",
		Category:    "Random",
		Type:        "string",
		Example:     "550e8400",
		Dynamic:     true,
	},
	{
		Name:        VarTimestamp,
		Description: "Unix timestamp in seconds",
		Category:    "System",
		Type:        "number",
		Example:     "1707321600",
	},
	{
		Name:        VarTimestampMS,
		Description: "Unix timestamp in milliseconds",
		Category:    "System",
		Type:        "number",
		Example:     "1707321600000",
	},
	{
		Name:              VarUnsubscribeLink,
		Description:       "Unsubscribe link for recipient",
		Category:          "Email",
		Type:              "url",
		Example:           "https://example.com/unsubscribe?email=...",
		RequiresRecipient: true,
	},
	{
		Name:             VarCompanyName,
		Description:      "Company name from campaign settings",
		Category:         "Company",
		Type:             "string",
		Example:          "Acme Corporation",
		RequiresCampaign: true,
	},
	{
		Name:             VarCompanyAddress,
		Description:      "Company address from campaign settings",
		Category:         "Company",
		Type:             "string",
		Example:          "123 Main St, City, State 12345",
		RequiresCampaign: true,
	},
	{
		Name:             VarCompanyPhone,
		Description:      "Company phone from campaign settings",
		Category:         "Company",
		Type:             "string",
		Example:          "(555) 123-4567",
		RequiresCampaign: true,
	},
	{
		Name:             VarCompanyEmail,
		Description:      "Company email from campaign settings",
		Category:         "Company",
		Type:             "string",
		Example:          "info@example.com",
		RequiresCampaign: true,
	},
	{
		Name:             VarCompanyWebsite,
		Description:      "Company website from campaign settings",
		Category:         "Company",
		Type:             "url",
		Example:          "https://example.com",
		RequiresCampaign: true,
	},
	{
		Name:             VarSupportEmail,
		Description:      "Support email from campaign settings",
		Category:         "Company",
		Type:             "string",
		Example:          "support@example.com",
		RequiresCampaign: true,
	},
	{
		Name:             VarSupportPhone,
		Description:      "Support phone from campaign settings",
		Category:         "Company",
		Type:             "string",
		Example:          "(555) 999-8888",
		RequiresCampaign: true,
	},
}

func GetVariablesByCategory(category string) []Variable {
	result := make([]Variable, 0)
	for _, v := range AllVariables {
		if v.Category == category {
			result = append(result, v)
		}
	}
	return result
}

func GetRecipientVariables() []Variable {
	return GetVariablesByCategory("Recipient")
}

func GetDateTimeVariables() []Variable {
	return GetVariablesByCategory("Date/Time")
}

func GetBusinessVariables() []Variable {
	return GetVariablesByCategory("Business")
}

func GetRandomVariables() []Variable {
	return GetVariablesByCategory("Random")
}

func GetCompanyVariables() []Variable {
	return GetVariablesByCategory("Company")
}

func GetContactVariables() []Variable {
	return GetVariablesByCategory("Contact")
}

func GetEmailVariables() []Variable {
	return GetVariablesByCategory("Email")
}

func GetSystemVariables() []Variable {
	return GetVariablesByCategory("System")
}

func GetAllCategories() []string {
	categories := make(map[string]bool)
	for _, v := range AllVariables {
		categories[v.Category] = true
	}

	result := make([]string, 0, len(categories))
	for cat := range categories {
		result = append(result, cat)
	}
	return result
}

func GetVariableByName(name string) (Variable, bool) {
	for _, v := range AllVariables {
		if v.Name == name {
			return v, true
		}
	}
	return Variable{}, false
}

func IsValidVariable(name string) bool {
	_, exists := GetVariableByName(name)
	return exists
}

func GetVariableExample(name string) string {
	if v, exists := GetVariableByName(name); exists {
		return v.Example
	}
	return ""
}

func GetVariableDescription(name string) string {
	if v, exists := GetVariableByName(name); exists {
		return v.Description
	}
	return ""
}

func GetDynamicVariables() []Variable {
	result := make([]Variable, 0)
	for _, v := range AllVariables {
		if v.Dynamic {
			result = append(result, v)
		}
	}
	return result
}

func GetRequiresRecipientVariables() []Variable {
	result := make([]Variable, 0)
	for _, v := range AllVariables {
		if v.RequiresRecipient {
			result = append(result, v)
		}
	}
	return result
}

func GetRequiresCampaignVariables() []Variable {
	result := make([]Variable, 0)
	for _, v := range AllVariables {
		if v.RequiresCampaign {
			result = append(result, v)
		}
	}
	return result
}

func GetRequiresAccountVariables() []Variable {
	result := make([]Variable, 0)
	for _, v := range AllVariables {
		if v.RequiresAccount {
			result = append(result, v)
		}
	}
	return result
}

type VariableGroup struct {
	Category    string
	Description string
	Variables   []Variable
}

func GetVariableGroups() []VariableGroup {
	groups := []VariableGroup{
		{
			Category:    "Recipient",
			Description: "Personalization variables related to email recipients",
			Variables:   GetRecipientVariables(),
		},
		{
			Category:    "Date/Time",
			Description: "Current date and time in various formats",
			Variables:   GetDateTimeVariables(),
		},
		{
			Category:    "Business",
			Description: "Business document numbers and identifiers",
			Variables:   GetBusinessVariables(),
		},
		{
			Category:    "Random",
			Description: "Random generated values for unique content",
			Variables:   GetRandomVariables(),
		},
		{
			Category:    "Company",
			Description: "Company information from campaign settings",
			Variables:   GetCompanyVariables(),
		},
		{
			Category:    "Contact",
			Description: "Contact information variables",
			Variables:   GetContactVariables(),
		},
		{
			Category:    "Email",
			Description: "Email-specific variables",
			Variables:   GetEmailVariables(),
		},
		{
			Category:    "System",
			Description: "System-level variables",
			Variables:   GetSystemVariables(),
		},
	}

	return groups
}

func GetVariableCount() int {
	return len(AllVariables)
}

func GetVariableCountByCategory(category string) int {
	count := 0
	for _, v := range AllVariables {
		if v.Category == category {
			count++
		}
	}
	return count
}

type VariableStatistics struct {
	TotalVariables      int
	RecipientVariables  int
	DateTimeVariables   int
	BusinessVariables   int
	RandomVariables     int
	CompanyVariables    int
	DynamicVariables    int
	RequiresRecipient   int
	RequiresCampaign    int
	RequiresAccount     int
	Categories          []string
}

func GetVariableStatistics() VariableStatistics {
	return VariableStatistics{
		TotalVariables:     GetVariableCount(),
		RecipientVariables: GetVariableCountByCategory("Recipient"),
		DateTimeVariables:  GetVariableCountByCategory("Date/Time"),
		BusinessVariables:  GetVariableCountByCategory("Business"),
		RandomVariables:    GetVariableCountByCategory("Random"),
		CompanyVariables:   GetVariableCountByCategory("Company"),
		DynamicVariables:   len(GetDynamicVariables()),
		RequiresRecipient:  len(GetRequiresRecipientVariables()),
		RequiresCampaign:   len(GetRequiresCampaignVariables()),
		RequiresAccount:    len(GetRequiresAccountVariables()),
		Categories:         GetAllCategories(),
	}
}
