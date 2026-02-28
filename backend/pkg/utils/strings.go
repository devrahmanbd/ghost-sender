package utils

import (
	"crypto/rand"
	"encoding/hex"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
    htmlTagRegex   = regexp.MustCompile(`<[^>]*>`)
    whitespaceRegex = regexp.MustCompile(`\s+`)
    variableRegex  = regexp.MustCompile(`\{\{([^}]+)\}\}|\$\{([^}]+)\}`)
    slugRegex      = regexp.MustCompile(`[^a-z0-9]+`)
)


func IsEmpty(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

func IsNotEmpty(s string) bool {
	return !IsEmpty(s)
}

func DefaultIfEmpty(s, defaultValue string) string {
	if IsEmpty(s) {
		return defaultValue
	}
	return s
}

func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func TruncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func TruncateWords(s string, maxWords int) string {
	words := strings.Fields(s)
	if len(words) <= maxWords {
		return s
	}
	return strings.Join(words[:maxWords], " ") + "..."
}

func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func TitleCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = Capitalize(strings.ToLower(word))
		}
	}
	return strings.Join(words, " ")
}

func ToSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func ToCamelCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	words := strings.Fields(s)
	for i := 1; i < len(words); i++ {
		words[i] = Capitalize(words[i])
	}
	return strings.Join(words, "")
}

func ToKebabCase(s string) string {
	s = ToSnakeCase(s)
	return strings.ReplaceAll(s, "_", "-")
}

func Slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func StripHTML(s string) string {
	return htmlTagRegex.ReplaceAllString(s, "")
}

func EscapeHTML(s string) string {
	return html.EscapeString(s)
}

func UnescapeHTML(s string) string {
	return html.UnescapeString(s)
}

func SanitizeHTML(s string) string {
	s = strings.ReplaceAll(s, "<script", "&lt;script")
	s = strings.ReplaceAll(s, "</script>", "&lt;/script&gt;")
	s = strings.ReplaceAll(s, "javascript:", "")
	s = strings.ReplaceAll(s, "onerror=", "")
	s = strings.ReplaceAll(s, "onclick=", "")
	return s
}

func RemoveSpecialChars(s string) string {
	var result strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func RemoveWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func NormalizeWhitespace(s string) string {
	return whitespaceRegex.ReplaceAllString(strings.TrimSpace(s), " ")
}

func Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func ContainsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func ContainsAll(s string, substrs []string) bool {
	for _, substr := range substrs {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}

func StartsWithAny(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func EndsWithAny(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func ExtractVariables(template string) []string {
	matches := variableRegex.FindAllStringSubmatch(template, -1)
	vars := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		var varName string
		if match[1] != "" {
			varName = match[1]
		} else if match[2] != "" {
			varName = match[2]
		}

		if varName != "" && !seen[varName] {
			vars = append(vars, varName)
			seen[varName] = true
		}
	}

	return vars
}

func ReplaceVariables(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return result
}

func HasVariable(template, varName string) bool {
	return strings.Contains(template, "{{"+varName+"}}") || 
		   strings.Contains(template, "${"+varName+"}")
}

func WordWrap(s string, lineWidth int) string {
	if lineWidth <= 0 {
		return s
	}

	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}

	var result strings.Builder
	var currentLine strings.Builder
	
	for _, word := range words {
		if currentLine.Len() == 0 {
			currentLine.WriteString(word)
		} else if currentLine.Len()+1+len(word) <= lineWidth {
			currentLine.WriteString(" ")
			currentLine.WriteString(word)
		} else {
			result.WriteString(currentLine.String())
			result.WriteString("\n")
			currentLine.Reset()
			currentLine.WriteString(word)
		}
	}

	if currentLine.Len() > 0 {
		result.WriteString(currentLine.String())
	}

	return result.String()
}

func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func LevenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}

	if len(s1) == 0 {
		return len(s2)
	}

	if len(s2) == 0 {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}

	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,
				min(matrix[i][j-1]+1, matrix[i-1][j-1]+cost),
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

func SimilarityRatio(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	maxLen := max(len(s1), len(s2))
	if maxLen == 0 {
		return 1.0
	}

	distance := LevenshteinDistance(s1, s2)
	return 1.0 - float64(distance)/float64(maxLen)
}

func RandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func RandomAlphanumeric(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	rand.Read(b)
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

func Pad(s string, length int, padChar string) string {
	if len(s) >= length {
		return s
	}
	padding := strings.Repeat(padChar, length-len(s))
	return s + padding
}

func PadLeft(s string, length int, padChar string) string {
	if len(s) >= length {
		return s
	}
	padding := strings.Repeat(padChar, length-len(s))
	return padding + s
}

func Center(s string, width int) string {
	if len(s) >= width {
		return s
	}
	leftPad := (width - len(s)) / 2
	rightPad := width - len(s) - leftPad
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

func Split(s, sep string, limit int) []string {
	if limit <= 0 {
		return strings.Split(s, sep)
	}

	parts := strings.SplitN(s, sep, limit)
	return parts
}

func SplitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func Join(parts []string, sep string) string {
	return strings.Join(parts, sep)
}

func JoinNonEmpty(parts []string, sep string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if IsNotEmpty(part) {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, sep)
}

func RepeatString(s string, count int) string {
	return strings.Repeat(s, count)
}

func CountOccurrences(s, substr string) int {
	return strings.Count(s, substr)
}

func ReplaceFirst(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}

func ReplaceLast(s, old, new string) string {
	idx := strings.LastIndex(s, old)
	if idx == -1 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func RemovePrefix(s, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

func RemoveSuffix(s, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}

func EnsurePrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}

func EnsureSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s
	}
	return s + suffix
}


func IsAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) > 0
}

func IsNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func IsAlphanumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Indent(s string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	lines := SplitLines(s)
	for i, line := range lines {
		if IsNotEmpty(line) {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

func Dedent(s string) string {
	lines := SplitLines(s)
	if len(lines) == 0 {
		return s
	}

	minIndent := -1
	for _, line := range lines {
		if IsEmpty(line) {
			continue
		}
		indent := 0
		for _, r := range line {
			if r == ' ' || r == '\t' {
				indent++
			} else {
				break
			}
		}
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}

	if minIndent <= 0 {
		return s
	}

	for i, line := range lines {
		if len(line) >= minIndent {
			lines[i] = line[minIndent:]
		}
	}

	return strings.Join(lines, "\n")
}

func ToASCII(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r < 128 {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func ContainsOnlyASCII(s string) bool {
	for _, r := range s {
		if r >= 128 {
			return false
		}
	}
	return true
}
