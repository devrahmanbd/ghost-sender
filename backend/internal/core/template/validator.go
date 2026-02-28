package template

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"email-campaign-system/pkg/logger"
)

var (
	ErrValidationFailed = errors.New("validation failed")
	ErrContentTooLarge  = errors.New("content exceeds size limit")
	ErrDangerousContent = errors.New("dangerous content detected")
	ErrInvalidEncoding  = errors.New("invalid character encoding")
	ErrMalformedHTML    = errors.New("malformed HTML structure")
)

type Validator struct {
	log            logger.Logger
	config         *ValidatorConfig
	dangerousTags  map[string]bool
	dangerousAttrs map[string]bool
	allowedTags    map[string]bool
	allowedAttrs   map[string]bool
	scriptPattern  *regexp.Regexp
	eventPattern   *regexp.Regexp
	urlPattern     *regexp.Regexp
	mu             sync.RWMutex
}

type ValidatorConfig struct {
	MaxSizeBytes         int
	MaxImageCount        int
	MaxLinkCount         int
	MaxDepth             int
	MaxTextToHTMLRatio   float64
	MinTextToHTMLRatio   float64
	MaxLinkDensity       float64
	AllowExternalImages  bool
	AllowExternalLinks   bool
	AllowInlineStyles    bool
	AllowEmbeddedScripts bool
	AllowIframes         bool
	AllowForms           bool
	RequireAltTags       bool
	ValidateURLs         bool
	StrictMode           bool
	SanitizeContent      bool
	RemoveDangerousTags  bool
	RemoveDangerousAttrs bool
	WhitelistMode        bool
}

type ValidationResult struct {
	Valid         bool
	Errors        []ValidationError
	Warnings      []ValidationWarning
	Info          []ValidationInfo
	SanitizedHTML string
	Statistics    *ValidationStatistics
	SecurityScore float64
	QualityScore  float64
}

type ValidationError struct {
	Code     string
	Message  string
	Location string
	Severity string
}

type ValidationWarning struct {
	Code     string
	Message  string
	Location string
}

type ValidationInfo struct {
	Key   string
	Value interface{}
}

type ValidationStatistics struct {
	TotalSize         int
	HTMLSize          int
	TextSize          int
	ImageCount        int
	LinkCount         int
	ExternalLinkCount int
	InternalLinkCount int
	FormCount         int
	ScriptCount       int
	StyleCount        int
	IframeCount       int
	DangerousTagCount int
	TextToHTMLRatio   float64
	LinkDensity       float64
	MaxDepth          int
}

func NewValidator(log logger.Logger) *Validator {
	config := DefaultValidatorConfig()

	validator := &Validator{
		log:    log,
		config: config,
		dangerousTags: map[string]bool{
			"script": true, "iframe": true, "object": true,
			"embed": true, "applet": true, "meta": true,
			"link": true, "base": true,
		},
		dangerousAttrs: map[string]bool{
			"onclick": true, "onerror": true, "onload": true,
			"onmouseover": true, "onfocus": true, "onblur": true,
			"onchange": true, "onsubmit": true,
		},
		allowedTags: map[string]bool{
			"html": true, "head": true, "body": true, "title": true,
			"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
			"p": true, "div": true, "span": true, "br": true, "hr": true,
			"a": true, "img": true, "table": true, "tr": true, "td": true, "th": true,
			"ul": true, "ol": true, "li": true, "strong": true, "em": true, "b": true, "i": true,
			"u": true, "center": true, "font": true,
		},
		allowedAttrs: map[string]bool{
			"href": true, "src": true, "alt": true, "title": true,
			"width": true, "height": true, "style": true, "class": true,
			"id": true, "align": true, "valign": true, "border": true,
			"cellpadding": true, "cellspacing": true, "color": true,
		},
	}

	validator.initializePatterns()

	return validator
}

func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		MaxSizeBytes:         1024 * 1024,
		MaxImageCount:        50,
		MaxLinkCount:         100,
		MaxDepth:             50,
		MaxTextToHTMLRatio:   0.8,
		MinTextToHTMLRatio:   0.1,
		MaxLinkDensity:       0.3,
		AllowExternalImages:  true,
		AllowExternalLinks:   true,
		AllowInlineStyles:    true,
		AllowEmbeddedScripts: false,
		AllowIframes:         false,
		AllowForms:           false,
		RequireAltTags:       true,
		ValidateURLs:         true,
		StrictMode:           false,
		SanitizeContent:      true,
		RemoveDangerousTags:  true,
		RemoveDangerousAttrs: true,
		WhitelistMode:        false,
	}
}

func (v *Validator) initializePatterns() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.scriptPattern = regexp.MustCompile(`<script[^>]*>.*?</script>`)
	v.eventPattern = regexp.MustCompile(`on\w+\s*=`)
	v.urlPattern = regexp.MustCompile(`^https?://`)
}

func (v *Validator) Validate(htmlContent string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:      true,
		Errors:     make([]ValidationError, 0),
		Warnings:   make([]ValidationWarning, 0),
		Info:       make([]ValidationInfo, 0),
		Statistics: &ValidationStatistics{},
	}

	if htmlContent == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Code:     "EMPTY_CONTENT",
			Message:  "HTML content is empty",
			Severity: "error",
		})
		return result, ErrEmptyContent
	}

	if len(htmlContent) > v.config.MaxSizeBytes {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Code:     "SIZE_LIMIT_EXCEEDED",
			Message:  fmt.Sprintf("Content size %d exceeds limit %d", len(htmlContent), v.config.MaxSizeBytes),
			Severity: "error",
		})
		return result, ErrContentTooLarge
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Code:     "PARSE_ERROR",
			Message:  fmt.Sprintf("HTML parsing failed: %v", err),
			Severity: "error",
		})
		return result, ErrMalformedHTML
	}

	v.validateStructure(doc, result)
	v.validateContent(doc, result)
	v.detectDangerousTags(doc, result)
	v.validateLinks(doc, result)
	v.validateImages(doc, result)
	v.collectStatistics(doc, htmlContent, result)
	v.calculateScores(result)

	if v.config.SanitizeContent {
		result.SanitizedHTML = v.Sanitize(htmlContent)
	}

	if len(result.Errors) > 0 {
		result.Valid = false
	}

	return result, nil
}

func (v *Validator) validateStructure(node *html.Node, result *ValidationResult) {
	depth := v.calculateMaxDepth(node, 0)
	if depth > v.config.MaxDepth {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "DEEP_NESTING",
			Message: fmt.Sprintf("HTML nesting depth %d exceeds recommended limit %d", depth, v.config.MaxDepth),
		})
	}

	result.Statistics.MaxDepth = depth

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := n.Data

			if v.config.WhitelistMode {
				if !v.allowedTags[tag] {
					result.Errors = append(result.Errors, ValidationError{
						Code:     "DISALLOWED_TAG",
						Message:  fmt.Sprintf("Tag '%s' is not in whitelist", tag),
						Location: tag,
						Severity: "error",
					})
				}
			}

			for _, attr := range n.Attr {
				if v.config.WhitelistMode {
					if !v.allowedAttrs[attr.Key] {
						result.Warnings = append(result.Warnings, ValidationWarning{
							Code:    "DISALLOWED_ATTR",
							Message: fmt.Sprintf("Attribute '%s' is not in whitelist", attr.Key),
						})
					}
				}

				if v.dangerousAttrs[attr.Key] && !v.config.AllowEmbeddedScripts {
					result.Errors = append(result.Errors, ValidationError{
						Code:     "DANGEROUS_ATTRIBUTE",
						Message:  fmt.Sprintf("Dangerous attribute '%s' detected in tag '%s'", attr.Key, tag),
						Location: tag,
						Severity: "critical",
					})
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
}

func (v *Validator) validateContent(node *html.Node, result *ValidationResult) {
	hasScript := v.hasElement(node, "script")
	hasIframe := v.hasElement(node, "iframe")
	hasForm := v.hasElement(node, "form")

	if hasScript && !v.config.AllowEmbeddedScripts {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "SCRIPT_NOT_ALLOWED",
			Message:  "Script tags are not allowed",
			Severity: "critical",
		})
		result.Statistics.ScriptCount++
	}

	if hasIframe && !v.config.AllowIframes {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "IFRAME_NOT_ALLOWED",
			Message:  "Iframe tags are not allowed",
			Severity: "critical",
		})
		result.Statistics.IframeCount++
	}

	if hasForm && !v.config.AllowForms {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "FORM_DETECTED",
			Message: "Form tags detected in template",
		})
		result.Statistics.FormCount++
	}

	v.detectInlineScripts(node, result)
}

func (v *Validator) detectDangerousTags(node *html.Node, result *ValidationResult) {
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if v.dangerousTags[n.Data] {
				result.Errors = append(result.Errors, ValidationError{
					Code:     "DANGEROUS_TAG",
					Message:  fmt.Sprintf("Dangerous tag '%s' detected", n.Data),
					Location: n.Data,
					Severity: "critical",
				})
				result.Statistics.DangerousTagCount++
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
}

func (v *Validator) validateLinks(node *html.Node, result *ValidationResult) {
	linkCount := 0
	externalCount := 0
	internalCount := 0

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			linkCount++

			var href string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
					break
				}
			}

			if href == "" {
				result.Warnings = append(result.Warnings, ValidationWarning{
					Code:    "EMPTY_LINK",
					Message: "Link with empty href detected",
				})
			} else {
				isExternal := v.isExternalURL(href)
				if isExternal {
					externalCount++
					if !v.config.AllowExternalLinks {
						result.Warnings = append(result.Warnings, ValidationWarning{
							Code:    "EXTERNAL_LINK",
							Message: fmt.Sprintf("External link detected: %s", href),
						})
					}
				} else {
					internalCount++
				}

				if v.config.ValidateURLs && isExternal {
					if !v.isValidURL(href) {
						result.Warnings = append(result.Warnings, ValidationWarning{
							Code:    "INVALID_URL",
							Message: fmt.Sprintf("Invalid URL format: %s", href),
						})
					}
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)

	result.Statistics.LinkCount = linkCount
	result.Statistics.ExternalLinkCount = externalCount
	result.Statistics.InternalLinkCount = internalCount

	if linkCount > v.config.MaxLinkCount {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "TOO_MANY_LINKS",
			Message: fmt.Sprintf("Link count %d exceeds recommended limit %d", linkCount, v.config.MaxLinkCount),
		})
	}
}

func (v *Validator) validateImages(node *html.Node, result *ValidationResult) {
	imageCount := 0

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			imageCount++

			var src, alt string
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					src = attr.Val
				} else if attr.Key == "alt" {
					alt = attr.Val
				}
			}

			if src == "" {
				result.Errors = append(result.Errors, ValidationError{
					Code:     "MISSING_IMAGE_SRC",
					Message:  "Image tag without src attribute",
					Severity: "error",
				})
			}

			if alt == "" && v.config.RequireAltTags {
				result.Warnings = append(result.Warnings, ValidationWarning{
					Code:    "MISSING_ALT_TAG",
					Message: fmt.Sprintf("Image without alt attribute: %s", src),
				})
			}

			if v.isExternalURL(src) && !v.config.AllowExternalImages {
				result.Warnings = append(result.Warnings, ValidationWarning{
					Code:    "EXTERNAL_IMAGE",
					Message: fmt.Sprintf("External image detected: %s", src),
				})
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)

	result.Statistics.ImageCount = imageCount

	if imageCount > v.config.MaxImageCount {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "TOO_MANY_IMAGES",
			Message: fmt.Sprintf("Image count %d exceeds recommended limit %d", imageCount, v.config.MaxImageCount),
		})
	}
}

func (v *Validator) detectInlineScripts(node *html.Node, result *ValidationResult) {
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if v.eventPattern.MatchString(attr.Key + "=") {
					result.Errors = append(result.Errors, ValidationError{
						Code:     "INLINE_SCRIPT",
						Message:  fmt.Sprintf("Inline script detected in attribute '%s'", attr.Key),
						Location: n.Data,
						Severity: "critical",
					})
				}

				if attr.Key == "href" && strings.HasPrefix(strings.ToLower(attr.Val), "javascript:") {
					result.Errors = append(result.Errors, ValidationError{
						Code:     "JAVASCRIPT_PROTOCOL",
						Message:  "JavaScript protocol detected in href",
						Location: n.Data,
						Severity: "critical",
					})
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
}

func (v *Validator) collectStatistics(node *html.Node, htmlContent string, result *ValidationResult) {
	result.Statistics.TotalSize = len(htmlContent)
	result.Statistics.HTMLSize = len(htmlContent)

	textContent := v.extractPlainText(node)
	result.Statistics.TextSize = len(textContent)

	if result.Statistics.HTMLSize > 0 {
		result.Statistics.TextToHTMLRatio = float64(result.Statistics.TextSize) / float64(result.Statistics.HTMLSize)
	}

	if result.Statistics.TextSize > 0 {
		wordCount := len(strings.Fields(textContent))
		if wordCount > 0 {
			result.Statistics.LinkDensity = float64(result.Statistics.LinkCount) / float64(wordCount)
		}
	}

	if result.Statistics.TextToHTMLRatio > v.config.MaxTextToHTMLRatio {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "HIGH_TEXT_RATIO",
			Message: fmt.Sprintf("Text to HTML ratio %.2f exceeds %.2f", result.Statistics.TextToHTMLRatio, v.config.MaxTextToHTMLRatio),
		})
	}

	if result.Statistics.TextToHTMLRatio < v.config.MinTextToHTMLRatio {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "LOW_TEXT_RATIO",
			Message: fmt.Sprintf("Text to HTML ratio %.2f below %.2f", result.Statistics.TextToHTMLRatio, v.config.MinTextToHTMLRatio),
		})
	}

	if result.Statistics.LinkDensity > v.config.MaxLinkDensity {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:    "HIGH_LINK_DENSITY",
			Message: fmt.Sprintf("Link density %.2f exceeds %.2f", result.Statistics.LinkDensity, v.config.MaxLinkDensity),
		})
	}
}

func (v *Validator) calculateScores(result *ValidationResult) {
	securityScore := 100.0
	qualityScore := 100.0

	for _, err := range result.Errors {
		switch err.Severity {
		case "critical":
			securityScore -= 20.0
		case "error":
			securityScore -= 10.0
		}
		qualityScore -= 5.0
	}

	for range result.Warnings {
		qualityScore -= 2.0
	}

	if securityScore < 0 {
		securityScore = 0
	}
	if qualityScore < 0 {
		qualityScore = 0
	}

	result.SecurityScore = securityScore
	result.QualityScore = qualityScore
}

func (v *Validator) Sanitize(htmlContent string) string {
	if !v.config.SanitizeContent {
		return htmlContent
	}

	sanitized := htmlContent

	if v.config.RemoveDangerousTags {
		sanitized = v.removeDangerousTags(sanitized)
	}

	if v.config.RemoveDangerousAttrs {
		sanitized = v.removeDangerousAttrs(sanitized)
	}

	return sanitized
}

func (v *Validator) removeDangerousTags(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	var remove []*html.Node
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && v.dangerousTags[n.Data] {
			remove = append(remove, n)
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(doc)

	for _, n := range remove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}

	var buf strings.Builder
	html.Render(&buf, doc)
	return buf.String()
}

func (v *Validator) removeDangerousAttrs(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			filteredAttrs := make([]html.Attribute, 0)
			for _, attr := range n.Attr {
				if !v.dangerousAttrs[attr.Key] {
					if !strings.HasPrefix(strings.ToLower(attr.Val), "javascript:") {
						filteredAttrs = append(filteredAttrs, attr)
					}
				}
			}
			n.Attr = filteredAttrs
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(doc)

	var buf strings.Builder
	html.Render(&buf, doc)
	return buf.String()
}

func (v *Validator) hasElement(node *html.Node, tagName string) bool {
	if node == nil {
		return false
	}

	if node.Type == html.ElementNode && node.Data == tagName {
		return true
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if v.hasElement(child, tagName) {
			return true
		}
	}

	return false
}

func (v *Validator) calculateMaxDepth(node *html.Node, currentDepth int) int {
	if node == nil {
		return currentDepth
	}

	maxDepth := currentDepth

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		depth := v.calculateMaxDepth(child, currentDepth+1)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

func (v *Validator) extractPlainText(node *html.Node) string {
	var text strings.Builder

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}

		if n.Type == html.TextNode {
			content := strings.TrimSpace(n.Data)
			if content != "" {
				text.WriteString(content)
				text.WriteString(" ")
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return strings.TrimSpace(text.String())
}

func (v *Validator) isExternalURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}

	if strings.HasPrefix(urlStr, "#") || strings.HasPrefix(urlStr, "/") {
		return false
	}

	if strings.HasPrefix(urlStr, "mailto:") || strings.HasPrefix(urlStr, "tel:") {
		return false
	}

	return strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://")
}

func (v *Validator) isValidURL(urlStr string) bool {
	_, err := url.Parse(urlStr)
	return err == nil
}

func (v *Validator) SetConfig(config *ValidatorConfig) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.config = config

	v.log.Info("validator config updated")
}

func (v *Validator) GetConfig() *ValidatorConfig {
	v.mu.RLock()
	defer v.mu.RUnlock()

	configCopy := *v.config
	return &configCopy
}

func (v *Validator) QuickValidate(htmlContent string) bool {
	if htmlContent == "" {
		return false
	}

	if len(htmlContent) > v.config.MaxSizeBytes {
		return false
	}

	_, err := html.Parse(strings.NewReader(htmlContent))
	return err == nil
}

func (v *Validator) IsSecure(htmlContent string) bool {
	result, err := v.Validate(htmlContent)
	if err != nil {
		return false
	}

	return result.SecurityScore >= 80.0
}

func (v *Validator) GetSecurityScore(htmlContent string) float64 {
	result, err := v.Validate(htmlContent)
	if err != nil {
		return 0.0
	}

	return result.SecurityScore
}

func (v *Validator) GetQualityScore(htmlContent string) float64 {
	result, err := v.Validate(htmlContent)
	if err != nil {
		return 0.0
	}

	return result.QualityScore
}

func (v *Validator) AddAllowedTag(tag string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.allowedTags[tag] = true
}

func (v *Validator) RemoveAllowedTag(tag string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.allowedTags, tag)
}

func (v *Validator) AddDangerousTag(tag string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.dangerousTags[tag] = true
}

func (v *Validator) RemoveDangerousTag(tag string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.dangerousTags, tag)
}
