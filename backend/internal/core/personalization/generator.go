package personalization

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"email-campaign-system/pkg/logger"

	"github.com/google/uuid"
)

type Generator struct {
	log    logger.Logger
	config *GeneratorConfig
	mu     sync.Mutex
}

type GeneratorConfig struct {
	InvoicePrefix      string
	OrderPrefix        string
	TrackingPrefix     string
	ReferencePrefix    string
	ConfirmationPrefix string
	TransactionPrefix  string
	UseTimestamp       bool
	IncludeYear        bool
	PhoneCountryCode   string
	DefaultAreaCode    string
}

func NewGenerator(log logger.Logger) *Generator {
	return &Generator{
		log:    log,
		config: DefaultGeneratorConfig(),
	}
}

func DefaultGeneratorConfig() *GeneratorConfig {
	return &GeneratorConfig{
		InvoicePrefix:      "INV",
		OrderPrefix:        "ORD",
		TrackingPrefix:     "TRK",
		ReferencePrefix:    "REF",
		ConfirmationPrefix: "CNF",
		TransactionPrefix:  "TXN",
		UseTimestamp:       true,
		IncludeYear:        true,
		PhoneCountryCode:   "+1",
		DefaultAreaCode:    "555",
	}
}

func (g *Generator) GenerateInvoiceNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.config.IncludeYear {
		year := time.Now().Year()
		randomPart := g.generateSecureRandomNumeric(5)
		return fmt.Sprintf("%s-%d-%s", g.config.InvoicePrefix, year, randomPart)
	}

	randomPart := g.generateSecureRandomNumeric(8)
	return fmt.Sprintf("%s-%s", g.config.InvoicePrefix, randomPart)
}

func (g *Generator) GenerateOrderNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.config.IncludeYear {
		year := time.Now().Year()
		randomPart := g.generateSecureRandomNumeric(5)
		return fmt.Sprintf("%s-%d-%s", g.config.OrderPrefix, year, randomPart)
	}

	randomPart := g.generateSecureRandomNumeric(8)
	return fmt.Sprintf("%s-%s", g.config.OrderPrefix, randomPart)
}

func (g *Generator) GenerateTrackingNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	randomPart := g.generateSecureRandomNumeric(10)
	return fmt.Sprintf("%s-%s", g.config.TrackingPrefix, randomPart)
}

func (g *Generator) GenerateReferenceNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	alphaNumeric := g.generateSecureRandomAlphanumeric(8)
	return fmt.Sprintf("%s-%s", g.config.ReferencePrefix, strings.ToUpper(alphaNumeric))
}

func (g *Generator) GenerateConfirmationNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	alphaNumeric := g.generateSecureRandomAlphanumeric(8)
	return fmt.Sprintf("%s-%s", g.config.ConfirmationPrefix, strings.ToUpper(alphaNumeric))
}

func (g *Generator) GenerateTransactionID() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	hex := g.generateSecureRandomHex(8)
	return fmt.Sprintf("%s-%s", g.config.TransactionPrefix, strings.ToUpper(hex))
}

func (g *Generator) GeneratePhoneNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	areaCode := g.config.DefaultAreaCode
	exchange := g.generateSecureRandomNumeric(3)
	lineNumber := g.generateSecureRandomNumeric(4)

	return fmt.Sprintf("(%s) %s-%s", areaCode, exchange, lineNumber)
}

func (g *Generator) GeneratePhoneNumberUS() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	areaCode := g.generateRandomAreaCode()
	exchange := g.generateSecureRandomNumeric(3)
	lineNumber := g.generateSecureRandomNumeric(4)

	return fmt.Sprintf("+1 (%s) %s-%s", areaCode, exchange, lineNumber)
}

func (g *Generator) GeneratePhoneNumberIntl() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	countryCode := g.generateRandomCountryCode()
	areaCode := g.generateSecureRandomNumeric(2)
	exchange := g.generateSecureRandomNumeric(4)
	lineNumber := g.generateSecureRandomNumeric(4)

	return fmt.Sprintf("+%s %s %s %s", countryCode, areaCode, exchange, lineNumber)
}

func (g *Generator) GenerateRandomNumber(length int) string {
	if length <= 0 {
		length = 6
	}
	if length > 20 {
		length = 20
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generateSecureRandomNumeric(length)
}

func (g *Generator) GenerateRandomAlpha(length int) string {
	if length <= 0 {
		length = 8
	}
	if length > 50 {
		length = 50
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	return strings.ToUpper(g.generateSecureRandomAlpha(length))
}

func (g *Generator) GenerateRandomAlphanumeric(length int) string {
	if length <= 0 {
		length = 10
	}
	if length > 50 {
		length = 50
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	return strings.ToUpper(g.generateSecureRandomAlphanumeric(length))
}

func (g *Generator) GenerateUUID() string {
	return uuid.New().String()
}

func (g *Generator) GenerateUUIDShort() string {
	fullUUID := uuid.New().String()
	return strings.Split(fullUUID, "-")[0]
}

func (g *Generator) generateSecureRandomNumeric(length int) string {
	const digits = "0123456789"
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			result[i] = digits[i%len(digits)]
			continue
		}
		result[i] = digits[num.Int64()]
	}

	return string(result)
}

func (g *Generator) generateSecureRandomAlpha(length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			result[i] = letters[i%len(letters)]
			continue
		}
		result[i] = letters[num.Int64()]
	}

	return string(result)
}

func (g *Generator) generateSecureRandomAlphanumeric(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			result[i] = chars[i%len(chars)]
			continue
		}
		result[i] = chars[num.Int64()]
	}

	return string(result)
}

func (g *Generator) generateSecureRandomHex(length int) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(hexChars))))
		if err != nil {
			result[i] = hexChars[i%len(hexChars)]
			continue
		}
		result[i] = hexChars[num.Int64()]
	}

	return string(result)
}

func (g *Generator) generateRandomAreaCode() string {
	validAreaCodes := []string{
		"201", "202", "203", "205", "206", "207", "208", "209",
		"210", "212", "213", "214", "215", "216", "217", "218",
		"301", "302", "303", "304", "305", "307", "308", "309",
		"310", "312", "313", "314", "315", "316", "317", "318",
		"401", "402", "403", "404", "405", "406", "407", "408",
		"409", "410", "412", "413", "414", "415", "417", "419",
		"501", "502", "503", "504", "505", "507", "508", "509",
		"510", "512", "513", "515", "516", "517", "518", "520",
		"601", "602", "603", "605", "606", "607", "608", "609",
		"610", "612", "614", "615", "616", "617", "618", "619",
		"701", "702", "703", "704", "706", "707", "708", "712",
		"713", "714", "715", "716", "717", "718", "719", "720",
		"801", "802", "803", "804", "805", "806", "808", "810",
		"812", "813", "814", "815", "816", "817", "818", "828",
		"901", "903", "904", "906", "907", "908", "909", "910",
		"912", "913", "914", "915", "916", "917", "918", "919",
	}

	num, err := rand.Int(rand.Reader, big.NewInt(int64(len(validAreaCodes))))
	if err != nil {
		return "555"
	}

	return validAreaCodes[num.Int64()]
}

func (g *Generator) generateRandomCountryCode() string {
	countryCodes := []string{
		"1", "7", "20", "27", "30", "31", "32", "33", "34", "36",
		"39", "40", "41", "43", "44", "45", "46", "47", "48", "49",
		"51", "52", "53", "54", "55", "56", "57", "58", "60", "61",
		"62", "63", "64", "65", "66", "81", "82", "84", "86", "90",
		"91", "92", "93", "94", "95", "98",
	}

	num, err := rand.Int(rand.Reader, big.NewInt(int64(len(countryCodes))))
	if err != nil {
		return "44"
	}

	return countryCodes[num.Int64()]
}

func (g *Generator) GenerateAccountNumber(length int) string {
	if length <= 0 {
		length = 10
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generateSecureRandomNumeric(length)
}

func (g *Generator) GenerateRoutingNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generateSecureRandomNumeric(9)
}

func (g *Generator) GenerateCreditCardNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	part1 := g.generateSecureRandomNumeric(4)
	part2 := g.generateSecureRandomNumeric(4)
	part3 := g.generateSecureRandomNumeric(4)
	part4 := g.generateSecureRandomNumeric(4)

	return fmt.Sprintf("%s-%s-%s-%s", part1, part2, part3, part4)
}

func (g *Generator) GenerateCVV() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generateSecureRandomNumeric(3)
}

func (g *Generator) GenerateSSN() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	part1 := g.generateSecureRandomNumeric(3)
	part2 := g.generateSecureRandomNumeric(2)
	part3 := g.generateSecureRandomNumeric(4)

	return fmt.Sprintf("%s-%s-%s", part1, part2, part3)
}

func (g *Generator) GenerateZipCode() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generateSecureRandomNumeric(5)
}

func (g *Generator) GenerateZipCodePlus4() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	part1 := g.generateSecureRandomNumeric(5)
	part2 := g.generateSecureRandomNumeric(4)

	return fmt.Sprintf("%s-%s", part1, part2)
}

func (g *Generator) GenerateIPAddress() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	octet1 := g.generateRandomOctet()
	octet2 := g.generateRandomOctet()
	octet3 := g.generateRandomOctet()
	octet4 := g.generateRandomOctet()

	return fmt.Sprintf("%d.%d.%d.%d", octet1, octet2, octet3, octet4)
}

func (g *Generator) generateRandomOctet() int {
	num, err := rand.Int(rand.Reader, big.NewInt(256))
	if err != nil {
		return 192
	}
	return int(num.Int64())
}

func (g *Generator) GenerateMACAddress() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = g.generateSecureRandomHex(2)
	}

	return strings.ToUpper(strings.Join(parts, ":"))
}

func (g *Generator) GenerateSerialNumber() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	prefix := g.generateSecureRandomAlpha(2)
	numeric := g.generateSecureRandomNumeric(8)
	suffix := g.generateSecureRandomAlpha(2)

	return strings.ToUpper(fmt.Sprintf("%s-%s-%s", prefix, numeric, suffix))
}

func (g *Generator) GenerateLicenseKey() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	part1 := g.generateSecureRandomAlphanumeric(5)
	part2 := g.generateSecureRandomAlphanumeric(5)
	part3 := g.generateSecureRandomAlphanumeric(5)
	part4 := g.generateSecureRandomAlphanumeric(5)

	return strings.ToUpper(fmt.Sprintf("%s-%s-%s-%s", part1, part2, part3, part4))
}

func (g *Generator) GenerateActivationCode() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return strings.ToUpper(g.generateSecureRandomAlphanumeric(16))
}

func (g *Generator) GenerateToken(length int) string {
	if length <= 0 {
		length = 32
	}
	if length > 128 {
		length = 128
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generateSecureRandomHex(length)
}

func (g *Generator) SetConfig(config *GeneratorConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.config = config
}

func (g *Generator) GetConfig() *GeneratorConfig {
	g.mu.Lock()
	defer g.mu.Unlock()

	configCopy := *g.config
	return &configCopy
}

type GeneratedValue struct {
	Type      string
	Value     string
	Timestamp time.Time
}

func (g *Generator) GenerateBatch(valueType string, count int) []GeneratedValue {
	results := make([]GeneratedValue, 0, count)

	for i := 0; i < count; i++ {
		var value string

		switch valueType {
		case "invoice":
			value = g.GenerateInvoiceNumber()
		case "order":
			value = g.GenerateOrderNumber()
		case "tracking":
			value = g.GenerateTrackingNumber()
		case "reference":
			value = g.GenerateReferenceNumber()
		case "confirmation":
			value = g.GenerateConfirmationNumber()
		case "transaction":
			value = g.GenerateTransactionID()
		case "uuid":
			value = g.GenerateUUID()
		case "phone":
			value = g.GeneratePhoneNumber()
		default:
			value = g.GenerateUUID()
		}

		results = append(results, GeneratedValue{
			Type:      valueType,
			Value:     value,
			Timestamp: time.Now(),
		})
	}

	return results
}
