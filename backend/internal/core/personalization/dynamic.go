package personalization

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"email-campaign-system/pkg/logger"
)

var (
	ErrInvalidRandomFormat = errors.New("invalid random variable format")
	ErrInvalidDateFormat   = errors.New("invalid date format")
	ErrInvalidParameter    = errors.New("invalid parameter")
	ErrUnsupportedVariable = errors.New("unsupported dynamic variable")
)

// DateTimeFormatter handles date/time formatting operations
type DateTimeFormatter struct {
	log      logger.Logger
	timezone string
	location *time.Location
}

// NewDateTimeFormatter creates a new DateTimeFormatter
func NewDateTimeFormatter(log logger.Logger, timezone string) *DateTimeFormatter {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	return &DateTimeFormatter{
		log:      log,
		timezone: timezone,
		location: loc,
	}
}

// FormatDate formats a date according to the given format
func (dtf *DateTimeFormatter) FormatDate(t time.Time, format string) string {
	return t.In(dtf.location).Format(format)
}

type DynamicProcessor struct {
	log       logger.Logger
	generator *Generator
	formatter *DateTimeFormatter
}

type DynamicVariablePattern struct {
	Pattern     *regexp.Regexp
	Processor   func(string, *PersonalizationContext) (string, error)
	Description string
}

func NewDynamicProcessor(log logger.Logger) *DynamicProcessor {
	return &DynamicProcessor{
		log:       log,
		generator: NewGenerator(log),
		formatter: NewDateTimeFormatter(log, "UTC"),
	}
}

func (dp *DynamicProcessor) ProcessRandomVariable(varName string, ctx *PersonalizationContext) (string, error) {
	if strings.HasPrefix(varName, "RANDOM_NUM_") {
		return dp.processRandomNumeric(varName)
	}

	if strings.HasPrefix(varName, "RANDOM_ALPHA_") {
		return dp.processRandomAlpha(varName)
	}

	if strings.HasPrefix(varName, "RANDOM_ALPHANUM_") {
		return dp.processRandomAlphanumeric(varName)
	}

	if strings.HasPrefix(varName, "RANDOM_HEX_") {
		return dp.processRandomHex(varName)
	}

	switch varName {
	case "RANDOM_NUM":
		return dp.generator.GenerateRandomNumber(6), nil
	case "RANDOM_ALPHA":
		return dp.generator.GenerateRandomAlpha(8), nil
	case "RANDOM_ALPHANUM":
		return dp.generator.GenerateRandomAlphanumeric(10), nil
	case "RANDOM_NUM_4":
		return dp.generator.GenerateRandomNumber(4), nil
	case "RANDOM_NUM_6":
		return dp.generator.GenerateRandomNumber(6), nil
	case "RANDOM_NUM_8":
		return dp.generator.GenerateRandomNumber(8), nil
	case "RANDOM_NUM_10":
		return dp.generator.GenerateRandomNumber(10), nil
	case "RANDOM_ALPHA_4":
		return dp.generator.GenerateRandomAlpha(4), nil
	case "RANDOM_ALPHA_6":
		return dp.generator.GenerateRandomAlpha(6), nil
	case "RANDOM_ALPHA_8":
		return dp.generator.GenerateRandomAlpha(8), nil
	case "RANDOM_ALPHA_10":
		return dp.generator.GenerateRandomAlpha(10), nil
	default:
		return "", ErrUnsupportedVariable
	}
}

func (dp *DynamicProcessor) processRandomNumeric(varName string) (string, error) {
	lengthStr := strings.TrimPrefix(varName, "RANDOM_NUM_")
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > 50 {
		return "", ErrInvalidRandomFormat
	}

	return dp.generator.GenerateRandomNumber(length), nil
}

func (dp *DynamicProcessor) processRandomAlpha(varName string) (string, error) {
	lengthStr := strings.TrimPrefix(varName, "RANDOM_ALPHA_")
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > 50 {
		return "", ErrInvalidRandomFormat
	}

	return dp.generator.GenerateRandomAlpha(length), nil
}

func (dp *DynamicProcessor) processRandomAlphanumeric(varName string) (string, error) {
	lengthStr := strings.TrimPrefix(varName, "RANDOM_ALPHANUM_")
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > 50 {
		return "", ErrInvalidRandomFormat
	}

	return dp.generator.GenerateRandomAlphanumeric(length), nil
}

func (dp *DynamicProcessor) processRandomHex(varName string) (string, error) {
	lengthStr := strings.TrimPrefix(varName, "RANDOM_HEX_")
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > 50 {
		return "", ErrInvalidRandomFormat
	}

	return dp.generateRandomHex(length), nil
}

func (dp *DynamicProcessor) generateRandomHex(length int) string {
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

	return strings.ToUpper(string(result))
}

func (dp *DynamicProcessor) ProcessDynamicVariable(varName string, ctx *PersonalizationContext) (string, error) {
	if strings.HasPrefix(varName, "RANDOM_") {
		return dp.ProcessRandomVariable(varName, ctx)
	}

	if strings.Contains(varName, "_DATE_") && (strings.Contains(varName, "+") || strings.Contains(varName, "-")) {
		return dp.ProcessDateWithOffset(varName, ctx)
	}

	if strings.HasPrefix(varName, "CUSTOM_") {
		return dp.processCustomDynamicVariable(varName, ctx)
	}

	if strings.HasPrefix(varName, "RANGE_") {
		return dp.processRangeVariable(varName, ctx)
	}

	return "", ErrUnsupportedVariable
}

func (dp *DynamicProcessor) ProcessDateWithOffset(varName string, ctx *PersonalizationContext) (string, error) {
	parts := dp.parseDateVariableWithOffset(varName)
	if parts == nil {
		return "", ErrInvalidDateFormat
	}

	baseDate := ctx.Timestamp
	if baseDate.IsZero() {
		baseDate = time.Now()
	}

	offsetDays := parts.Offset
	targetDate := baseDate.AddDate(0, 0, offsetDays)

	format := dp.getDateFormatFromVariableName(parts.BaseName)

	return dp.formatter.FormatDate(targetDate, format), nil
}

type DateVariableParts struct {
	BaseName string
	Offset   int
	Original string
}

func (dp *DynamicProcessor) parseDateVariableWithOffset(varName string) *DateVariableParts {
	pattern := regexp.MustCompile(`^([A-Z_]+_DATE)([+-]\d+)$`)
	matches := pattern.FindStringSubmatch(varName)

	if len(matches) != 3 {
		return nil
	}

	offset, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil
	}

	return &DateVariableParts{
		BaseName: matches[1],
		Offset:   offset,
		Original: varName,
	}
}

func (dp *DynamicProcessor) getDateFormatFromVariableName(baseName string) string {
	switch baseName {
	case "TODAY_DATE", "CURRENT_DATE", "CUSTOM_DATE":
		return "2006-01-02"
	case "TODAY_DATE_LONG", "CURRENT_DATE_LONG":
		return "January 2, 2006"
	case "TODAY_DATE_SHORT", "CURRENT_DATE_SHORT":
		return "01/02/2006"
	case "TODAY_DATE_MEDIUM", "CURRENT_DATE_MEDIUM":
		return "Jan 2, 2006"
	default:
		return "2006-01-02"
	}
}

func (dp *DynamicProcessor) processCustomDynamicVariable(varName string, ctx *PersonalizationContext) (string, error) {
	if strings.HasPrefix(varName, "CUSTOM_NUM_") {
		return dp.processCustomNumeric(varName, ctx)
	}

	if strings.HasPrefix(varName, "CUSTOM_ALPHA_") {
		return dp.processCustomAlpha(varName, ctx)
	}

	if strings.HasPrefix(varName, "CUSTOM_DATE") {
		return dp.processCustomDate(varName, ctx)
	}

	if ctx.CustomFields != nil {
		if value, exists := ctx.CustomFields[varName]; exists {
			return value, nil
		}
	}

	return "", ErrUnsupportedVariable
}

func (dp *DynamicProcessor) processCustomNumeric(varName string, ctx *PersonalizationContext) (string, error) {
	lengthStr := strings.TrimPrefix(varName, "CUSTOM_NUM_")
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > 20 {
		return "", ErrInvalidParameter
	}

	return dp.generator.GenerateRandomNumber(length), nil
}

func (dp *DynamicProcessor) processCustomAlpha(varName string, ctx *PersonalizationContext) (string, error) {
	lengthStr := strings.TrimPrefix(varName, "CUSTOM_ALPHA_")
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > 20 {
		return "", ErrInvalidParameter
	}

	return dp.generator.GenerateRandomAlpha(length), nil
}

func (dp *DynamicProcessor) processCustomDate(varName string, ctx *PersonalizationContext) (string, error) {
	baseDate := ctx.Timestamp
	if baseDate.IsZero() {
		baseDate = time.Now()
	}

	if strings.Contains(varName, "+") || strings.Contains(varName, "-") {
		return dp.ProcessDateWithOffset(varName, ctx)
	}

	return dp.formatter.FormatDate(baseDate, "2006-01-02"), nil
}

func (dp *DynamicProcessor) processRangeVariable(varName string, ctx *PersonalizationContext) (string, error) {
	pattern := regexp.MustCompile(`^RANGE_(\d+)_(\d+)$`)
	matches := pattern.FindStringSubmatch(varName)

	if len(matches) != 3 {
		return "", ErrInvalidParameter
	}

	min, err1 := strconv.Atoi(matches[1])
	max, err2 := strconv.Atoi(matches[2])

	if err1 != nil || err2 != nil || min >= max {
		return "", ErrInvalidParameter
	}

	rangeSize := max - min + 1
	num, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))
	if err != nil {
		return fmt.Sprintf("%d", min), nil
	}

	return fmt.Sprintf("%d", min+int(num.Int64())), nil
}

func (dp *DynamicProcessor) GenerateSequentialNumber(prefix string, sequence int) string {
	return fmt.Sprintf("%s-%06d", prefix, sequence)
}

func (dp *DynamicProcessor) GenerateWeightedRandom(options map[string]int) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options provided")
	}

	totalWeight := 0
	for _, weight := range options {
		totalWeight += weight
	}

	if totalWeight <= 0 {
		return "", errors.New("invalid total weight")
	}

	randomNum, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight)))
	if err != nil {
		for key := range options {
			return key, nil
		}
	}

	currentWeight := int64(0)
	targetWeight := randomNum.Int64()

	for option, weight := range options {
		currentWeight += int64(weight)
		if currentWeight > targetWeight {
			return option, nil
		}
	}

	for key := range options {
		return key, nil
	}

	return "", errors.New("failed to select weighted option")
}

func (dp *DynamicProcessor) ProcessConditionalVariable(varName string, ctx *PersonalizationContext) (string, error) {
	if strings.HasPrefix(varName, "IF_") {
		return dp.processIfCondition(varName, ctx)
	}

	return "", ErrUnsupportedVariable
}

func (dp *DynamicProcessor) processIfCondition(varName string, ctx *PersonalizationContext) (string, error) {
	return "", ErrUnsupportedVariable
}

func (dp *DynamicProcessor) ParseVariableParameters(varName string) map[string]string {
	params := make(map[string]string)

	if strings.Contains(varName, "_") {
		parts := strings.Split(varName, "_")
		if len(parts) > 1 {
			params["prefix"] = parts[0]
			params["suffix"] = strings.Join(parts[1:], "_")
		}
	}

	if strings.Contains(varName, "+") || strings.Contains(varName, "-") {
		offsetPattern := regexp.MustCompile(`([+-]\d+)`)
		matches := offsetPattern.FindStringSubmatch(varName)
		if len(matches) > 0 {
			params["offset"] = matches[1]
		}
	}

	digitPattern := regexp.MustCompile(`\d+`)
	matches := digitPattern.FindAllString(varName, -1)
	if len(matches) > 0 {
		params["length"] = matches[0]
	}

	return params
}

func (dp *DynamicProcessor) ValidateDynamicVariable(varName string) error {
	if strings.HasPrefix(varName, "RANDOM_NUM_") {
		lengthStr := strings.TrimPrefix(varName, "RANDOM_NUM_")
		length, err := strconv.Atoi(lengthStr)
		if err != nil || length <= 0 || length > 50 {
			return ErrInvalidRandomFormat
		}
		return nil
	}

	if strings.HasPrefix(varName, "RANDOM_ALPHA_") {
		lengthStr := strings.TrimPrefix(varName, "RANDOM_ALPHA_")
		length, err := strconv.Atoi(lengthStr)
		if err != nil || length <= 0 || length > 50 {
			return ErrInvalidRandomFormat
		}
		return nil
	}

	if strings.HasPrefix(varName, "RANDOM_ALPHANUM_") {
		lengthStr := strings.TrimPrefix(varName, "RANDOM_ALPHANUM_")
		length, err := strconv.Atoi(lengthStr)
		if err != nil || length <= 0 || length > 50 {
			return ErrInvalidRandomFormat
		}
		return nil
	}

	if strings.Contains(varName, "_DATE_") && (strings.Contains(varName, "+") || strings.Contains(varName, "-")) {
		parts := dp.parseDateVariableWithOffset(varName)
		if parts == nil {
			return ErrInvalidDateFormat
		}
		return nil
	}

	return nil
}

func (dp *DynamicProcessor) GetSupportedPatterns() []DynamicVariablePattern {
	return []DynamicVariablePattern{
		{
			Pattern:     regexp.MustCompile(`^RANDOM_NUM_\d+$`),
			Description: "Random numeric string with specified length",
		},
		{
			Pattern:     regexp.MustCompile(`^RANDOM_ALPHA_\d+$`),
			Description: "Random alphabetic string with specified length",
		},
		{
			Pattern:     regexp.MustCompile(`^RANDOM_ALPHANUM_\d+$`),
			Description: "Random alphanumeric string with specified length",
		},
		{
			Pattern:     regexp.MustCompile(`^RANDOM_HEX_\d+$`),
			Description: "Random hexadecimal string with specified length",
		},
		{
			Pattern:     regexp.MustCompile(`^[A-Z_]+_DATE[+-]\d+$`),
			Description: "Date with offset in days",
		},
		{
			Pattern:     regexp.MustCompile(`^CUSTOM_NUM_\d+$`),
			Description: "Custom numeric with specified length",
		},
		{
			Pattern:     regexp.MustCompile(`^CUSTOM_ALPHA_\d+$`),
			Description: "Custom alphabetic with specified length",
		},
		{
			Pattern:     regexp.MustCompile(`^RANGE_\d+_\d+$`),
			Description: "Random number within specified range",
		},
	}
}

func (dp *DynamicProcessor) IsDynamicVariable(varName string) bool {
	patterns := dp.GetSupportedPatterns()
	for _, pattern := range patterns {
		if pattern.Pattern.MatchString(varName) {
			return true
		}
	}
	return false
}

type DynamicVariableInfo struct {
	Name       string
	Type       string
	Parameters map[string]string
	Example    string
	IsValid    bool
	Error      error
}

func (dp *DynamicProcessor) AnalyzeDynamicVariable(varName string) *DynamicVariableInfo {
	info := &DynamicVariableInfo{
		Name:       varName,
		Parameters: dp.ParseVariableParameters(varName),
	}

	if strings.HasPrefix(varName, "RANDOM_NUM_") {
		info.Type = "random_numeric"
		info.Example = "123456"
	} else if strings.HasPrefix(varName, "RANDOM_ALPHA_") {
		info.Type = "random_alpha"
		info.Example = "ABCDEF"
	} else if strings.HasPrefix(varName, "RANDOM_ALPHANUM_") {
		info.Type = "random_alphanumeric"
		info.Example = "A1B2C3"
	} else if strings.Contains(varName, "_DATE_") {
		info.Type = "date_offset"
		info.Example = "2026-02-14"
	} else if strings.HasPrefix(varName, "RANGE_") {
		info.Type = "range"
		info.Example = "50"
	} else {
		info.Type = "unknown"
	}

	err := dp.ValidateDynamicVariable(varName)
	info.IsValid = err == nil
	info.Error = err

	return info
}

func (dp *DynamicProcessor) SetGenerator(generator *Generator) {
	dp.generator = generator
}

func (dp *DynamicProcessor) SetDateTimeFormatter(formatter *DateTimeFormatter) {
	dp.formatter = formatter
}
