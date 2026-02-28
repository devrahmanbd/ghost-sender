package notification

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

type MessageFormatter struct {
	defaultFormat FormatType
	maxLength     int
	truncateSuffix string
}

type FormatType string

const (
	FormatTypePlain      FormatType = "plain"
	FormatTypeMarkdown   FormatType = "markdown"
	FormatTypeMarkdownV2 FormatType = "markdownv2"
	FormatTypeHTML       FormatType = "html"
)

type FormattedMessage struct {
	Content   string
	Format    FormatType
	Truncated bool
	Length    int
}

type FormatOptions struct {
	Format         FormatType
	MaxLength      int
	TruncateSuffix string
	EscapeSpecial  bool
	PreserveSpaces bool
}

func NewMessageFormatter() *MessageFormatter {
	return &MessageFormatter{
		defaultFormat:  FormatTypeHTML,
		maxLength:      4096,
		truncateSuffix: "...",
	}
}

func (mf *MessageFormatter) Format(message string, format FormatType) (*FormattedMessage, error) {
	return mf.FormatWithOptions(message, &FormatOptions{
		Format:         format,
		MaxLength:      mf.maxLength,
		TruncateSuffix: mf.truncateSuffix,
		EscapeSpecial:  true,
	})
}

func (mf *MessageFormatter) FormatWithOptions(message string, opts *FormatOptions) (*FormattedMessage, error) {
	if opts == nil {
		opts = &FormatOptions{
			Format:         mf.defaultFormat,
			MaxLength:      mf.maxLength,
			TruncateSuffix: mf.truncateSuffix,
			EscapeSpecial:  true,
		}
	}

	formatted := message
	truncated := false

	if opts.EscapeSpecial {
		formatted = mf.escape(formatted, opts.Format)
	}

	if opts.MaxLength > 0 && len(formatted) > opts.MaxLength {
		suffixLen := len(opts.TruncateSuffix)
		formatted = formatted[:opts.MaxLength-suffixLen] + opts.TruncateSuffix
		truncated = true
	}

	return &FormattedMessage{
		Content:   formatted,
		Format:    opts.Format,
		Truncated: truncated,
		Length:    len(formatted),
	}, nil
}

func (mf *MessageFormatter) escape(message string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown:
		return mf.escapeMarkdown(message)
	case FormatTypeMarkdownV2:
		return mf.escapeMarkdownV2(message)
	case FormatTypeHTML:
		return mf.escapeHTML(message)
	default:
		return message
	}
}

func (mf *MessageFormatter) escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return replacer.Replace(text)
}

func (mf *MessageFormatter) escapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

func (mf *MessageFormatter) escapeHTML(text string) string {
	return html.EscapeString(text)
}

func (mf *MessageFormatter) Bold(text string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown, FormatTypeMarkdownV2:
		return fmt.Sprintf("*%s*", text)
	case FormatTypeHTML:
		return fmt.Sprintf("<b>%s</b>", text)
	default:
		return text
	}
}

func (mf *MessageFormatter) Italic(text string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown, FormatTypeMarkdownV2:
		return fmt.Sprintf("_%s_", text)
	case FormatTypeHTML:
		return fmt.Sprintf("<i>%s</i>", text)
	default:
		return text
	}
}

func (mf *MessageFormatter) Code(text string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown, FormatTypeMarkdownV2:
		return fmt.Sprintf("`%s`", text)
	case FormatTypeHTML:
		return fmt.Sprintf("<code>%s</code>", text)
	default:
		return text
	}
}

func (mf *MessageFormatter) CodeBlock(text string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown, FormatTypeMarkdownV2:
		return fmt.Sprintf("```\n%s\n```", text)
	case FormatTypeHTML:
		return fmt.Sprintf("<pre>%s</pre>", text)
	default:
		return text
	}
}

func (mf *MessageFormatter) Link(text, url string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown, FormatTypeMarkdownV2:
		return fmt.Sprintf("[%s](%s)", text, url)
	case FormatTypeHTML:
		return fmt.Sprintf("<a href=\"%s\">%s</a>", url, text)
	default:
		return fmt.Sprintf("%s: %s", text, url)
	}
}

func (mf *MessageFormatter) Strikethrough(text string, format FormatType) string {
	switch format {
	case FormatTypeMarkdownV2:
		return fmt.Sprintf("~%s~", text)
	case FormatTypeHTML:
		return fmt.Sprintf("<s>%s</s>", text)
	default:
		return text
	}
}

func (mf *MessageFormatter) Underline(text string, format FormatType) string {
	switch format {
	case FormatTypeHTML:
		return fmt.Sprintf("<u>%s</u>", text)
	default:
		return text
	}
}

func (mf *MessageFormatter) FormatList(items []string, format FormatType, ordered bool) string {
	var builder strings.Builder

	for i, item := range items {
		if ordered {
			builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
		} else {
			builder.WriteString(fmt.Sprintf("• %s\n", item))
		}
	}

	return strings.TrimSuffix(builder.String(), "\n")
}

func (mf *MessageFormatter) FormatTable(headers []string, rows [][]string, format FormatType) string {
	if format == FormatTypeHTML {
		return mf.formatHTMLTable(headers, rows)
	}
	return mf.formatPlainTable(headers, rows)
}

func (mf *MessageFormatter) formatHTMLTable(headers []string, rows [][]string) string {
	var builder strings.Builder

	builder.WriteString("<table>\n")

	if len(headers) > 0 {
		builder.WriteString("  <thead>\n    <tr>\n")
		for _, header := range headers {
			builder.WriteString(fmt.Sprintf("      <th>%s</th>\n", header))
		}
		builder.WriteString("    </tr>\n  </thead>\n")
	}

	builder.WriteString("  <tbody>\n")
	for _, row := range rows {
		builder.WriteString("    <tr>\n")
		for _, cell := range row {
			builder.WriteString(fmt.Sprintf("      <td>%s</td>\n", cell))
		}
		builder.WriteString("    </tr>\n")
	}
	builder.WriteString("  </tbody>\n")

	builder.WriteString("</table>")

	return builder.String()
}

func (mf *MessageFormatter) formatPlainTable(headers []string, rows [][]string) string {
	colWidths := make([]int, len(headers))

	for i, header := range headers {
		colWidths[i] = len(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	var builder strings.Builder

	if len(headers) > 0 {
		for i, header := range headers {
			builder.WriteString(fmt.Sprintf("%-*s", colWidths[i]+2, header))
		}
		builder.WriteString("\n")

		for _, width := range colWidths {
			builder.WriteString(strings.Repeat("-", width+2))
		}
		builder.WriteString("\n")
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				builder.WriteString(fmt.Sprintf("%-*s", colWidths[i]+2, cell))
			}
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func (mf *MessageFormatter) FormatMap(data map[string]interface{}, format FormatType) string {
	var builder strings.Builder

	for key, value := range data {
		formattedKey := mf.Bold(key, format)
		builder.WriteString(fmt.Sprintf("%s: %v\n", formattedKey, value))
	}

	return strings.TrimSuffix(builder.String(), "\n")
}

func (mf *MessageFormatter) FormatTime(t time.Time, layout string) string {
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	return t.Format(layout)
}

func (mf *MessageFormatter) FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

func (mf *MessageFormatter) FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (mf *MessageFormatter) FormatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1000000000)
}

func (mf *MessageFormatter) FormatPercentage(value, total float64) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.2f%%", (value/total)*100)
}

func (mf *MessageFormatter) StripFormatting(message string, format FormatType) string {
	switch format {
	case FormatTypeMarkdown, FormatTypeMarkdownV2:
		return mf.stripMarkdown(message)
	case FormatTypeHTML:
		return mf.stripHTML(message)
	default:
		return message
	}
}

func (mf *MessageFormatter) stripMarkdown(text string) string {
	patterns := []string{
		`\*\*([^*]+)\*\*`,
		`\*([^*]+)\*`,
		`__([^_]+)__`,
		`_([^_]+)_`,
		"`([^`]+)`",
		`\[([^\]]+)\]\([^\)]+\)`,
	}

	result := text
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "$1")
	}

	return result
}

func (mf *MessageFormatter) stripHTML(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(text, "")
}

func (mf *MessageFormatter) ConvertFormat(message string, from, to FormatType) string {
	stripped := mf.StripFormatting(message, from)
	return mf.escape(stripped, to)
}

func (mf *MessageFormatter) Sanitize(message string) string {
	message = strings.TrimSpace(message)

	re := regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	message = re.ReplaceAllString(message, "")

	re = regexp.MustCompile(`\n{3,}`)
	message = re.ReplaceAllString(message, "\n\n")

	re = regexp.MustCompile(` {2,}`)
	message = re.ReplaceAllString(message, " ")

	return message
}

func (mf *MessageFormatter) Truncate(message string, maxLength int, suffix string) string {
	if len(message) <= maxLength {
		return message
	}

	suffixLen := len(suffix)
	return message[:maxLength-suffixLen] + suffix
}

func (mf *MessageFormatter) WordWrap(message string, width int) string {
	if width <= 0 {
		return message
	}

	var result strings.Builder
	words := strings.Fields(message)
	lineLength := 0

	for i, word := range words {
		wordLen := len(word)

		if lineLength+wordLen+1 > width {
			result.WriteString("\n")
			lineLength = 0
		} else if i > 0 {
			result.WriteString(" ")
			lineLength++
		}

		result.WriteString(word)
		lineLength += wordLen
	}

	return result.String()
}

func (mf *MessageFormatter) AddEmoji(text string, emoji string) string {
	return emoji + " " + text
}

func (mf *MessageFormatter) FormatError(err error, format FormatType) string {
	errorText := mf.Bold("Error:", format) + " " + err.Error()
	return errorText
}

func (mf *MessageFormatter) FormatWarning(message string, format FormatType) string {
	return mf.AddEmoji(message, "⚠️")
}

func (mf *MessageFormatter) FormatSuccess(message string, format FormatType) string {
	return mf.AddEmoji(message, "✅")
}

func (mf *MessageFormatter) FormatInfo(message string, format FormatType) string {
	return mf.AddEmoji(message, "ℹ️")
}

func DefaultFormatOptions() *FormatOptions {
	return &FormatOptions{
		Format:         FormatTypeHTML,
		MaxLength:      4096,
		TruncateSuffix: "...",
		EscapeSpecial:  true,
		PreserveSpaces: false,
	}
}

func FormatNotification(notification *Notification, format FormatType) (string, error) {
	formatter := NewMessageFormatter()

	var builder strings.Builder

	if notification.Title != "" {
		builder.WriteString(formatter.Bold(notification.Title, format))
		builder.WriteString("\n\n")
	}

	builder.WriteString(notification.Message)

	if len(notification.Data) > 0 {
		builder.WriteString("\n\n")
		builder.WriteString(formatter.FormatMap(notification.Data, format))
	}

	formatted, err := formatter.Format(builder.String(), format)
	if err != nil {
		return "", err
	}

	return formatted.Content, nil
}

