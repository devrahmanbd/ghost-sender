package template

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"email-campaign-system/pkg/logger"
)

var (
	ErrInvalidHTML    = errors.New("invalid HTML structure")
	ErrParsingFailed  = errors.New("HTML parsing failed")
	ErrEmptyContent   = errors.New("content is empty")
)

type Parser struct {
	log              logger.Logger
	config           *ParserConfig
	variablePatterns []*regexp.Regexp
	urlPattern       *regexp.Regexp
	emailPattern     *regexp.Regexp
	imagePattern     *regexp.Regexp
	linkPattern      *regexp.Regexp
	mu               sync.RWMutex
}

type ParserConfig struct {
	ExtractVariables     bool
	ExtractLinks         bool
	ExtractImages        bool
	ExtractText          bool
	ExtractMetadata      bool
	ValidateStructure    bool
	NormalizeWhitespace  bool
	PreserveFormatting   bool
	MaxDepth             int
	MaxNodes             int
}

type ParseResult struct {
	Variables       []string
	Links           []Link
	Images          []Image
	TextContent     string
	PlainText       string
	HTMLStructure   *HTMLNode
	Metadata        map[string]string
	WordCount       int
	CharCount       int
	NodeCount       int
	Depth           int
	HasForm         bool
	HasScript       bool
	HasStyle        bool
	ExternalLinks   int
	InternalLinks   int
	Errors          []string
	Warnings        []string
}

type Link struct {
	URL      string
	Text     string
	Title    string
	Rel      string
	Target   string
	IsExternal bool
}

type Image struct {
	URL    string
	Alt    string
	Title  string
	Width  string
	Height string
}

type HTMLNode struct {
	Tag        string
	Attributes map[string]string
	Text       string
	Children   []*HTMLNode
	Depth      int
}

func NewParser(log logger.Logger) *Parser {
	config := DefaultParserConfig()

	parser := &Parser{
		log:    log,
		config: config,
	}

	parser.initializePatterns()

	return parser
}

func DefaultParserConfig() *ParserConfig {
	return &ParserConfig{
		ExtractVariables:     true,
		ExtractLinks:         true,
		ExtractImages:        true,
		ExtractText:          true,
		ExtractMetadata:      true,
		ValidateStructure:    true,
		NormalizeWhitespace:  true,
		PreserveFormatting:   false,
		MaxDepth:             50,
		MaxNodes:             10000,
	}
}

func (p *Parser) initializePatterns() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.variablePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\{\{([A-Z_][A-Z0-9_]*(?:\|[a-z]+(?:\:[^}]+)?)?)\}\}`),
		regexp.MustCompile(`\{([A-Z_][A-Z0-9_]*)\}`),
		regexp.MustCompile(`\$([A-Z_][A-Z0-9_]*)\$`),
		regexp.MustCompile(`%([A-Z_][A-Z0-9_]*)%`),
	}

	p.urlPattern = regexp.MustCompile(`https?://[^\s<>"]+`)
	p.emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	p.imagePattern = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)
	p.linkPattern = regexp.MustCompile(`<a[^>]+href=["']([^"']+)["']`)
}

func (p *Parser) Parse(htmlContent string) (*ParseResult, error) {
	if htmlContent == "" {
		return nil, ErrEmptyContent
	}

	result := &ParseResult{
		Variables: make([]string, 0),
		Links:     make([]Link, 0),
		Images:    make([]Image, 0),
		Metadata:  make(map[string]string),
		Errors:    make([]string, 0),
		Warnings:  make([]string, 0),
	}

	if p.config.ExtractVariables {
		result.Variables = p.extractVariables(htmlContent)
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("HTML parsing error: %v", err))
		return result, ErrParsingFailed
	}

	if p.config.ValidateStructure {
		if errs := p.validateStructure(doc); len(errs) > 0 {
			result.Errors = append(result.Errors, errs...)
		}
	}

	result.HTMLStructure = p.buildNodeTree(doc, 0)
	result.NodeCount = p.countNodes(doc)
	result.Depth = p.calculateDepth(doc, 0)

	if p.config.ExtractLinks {
		result.Links = p.extractLinks(doc)
		for _, link := range result.Links {
			if link.IsExternal {
				result.ExternalLinks++
			} else {
				result.InternalLinks++
			}
		}
	}

	if p.config.ExtractImages {
		result.Images = p.extractImages(doc)
	}

	if p.config.ExtractText {
		result.TextContent = p.extractText(doc)
		result.PlainText = p.extractPlainText(doc)
		result.WordCount = p.countWords(result.PlainText)
		result.CharCount = len(result.PlainText)
	}

	if p.config.ExtractMetadata {
		result.Metadata = p.extractMetadata(doc)
	}

	result.HasForm = p.hasElement(doc, "form")
	result.HasScript = p.hasElement(doc, "script")
	result.HasStyle = p.hasElement(doc, "style")

	return result, nil
}

func (p *Parser) extractVariables(content string) []string {
	variableMap := make(map[string]bool)

	for _, pattern := range p.variablePatterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				varName := match[1]
				if strings.Contains(varName, "|") {
					varName = strings.Split(varName, "|")[0]
				}
				variableMap[varName] = true
			}
		}
	}

	variables := make([]string, 0, len(variableMap))
	for varName := range variableMap {
		variables = append(variables, varName)
	}

	return variables
}

func (p *Parser) extractLinks(node *html.Node) []Link {
	links := make([]Link, 0)

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			link := Link{}
			for _, attr := range n.Attr {
				switch attr.Key {
				case "href":
					link.URL = attr.Val
				case "title":
					link.Title = attr.Val
				case "rel":
					link.Rel = attr.Val
				case "target":
					link.Target = attr.Val
				}
			}

			link.Text = p.getNodeText(n)
			link.IsExternal = p.isExternalLink(link.URL)

			if link.URL != "" {
				links = append(links, link)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return links
}

func (p *Parser) extractImages(node *html.Node) []Image {
	images := make([]Image, 0)

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			image := Image{}
			for _, attr := range n.Attr {
				switch attr.Key {
				case "src":
					image.URL = attr.Val
				case "alt":
					image.Alt = attr.Val
				case "title":
					image.Title = attr.Val
				case "width":
					image.Width = attr.Val
				case "height":
					image.Height = attr.Val
				}
			}

			if image.URL != "" {
				images = append(images, image)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return images
}

func (p *Parser) extractText(node *html.Node) string {
	var text strings.Builder

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.TextNode {
			content := n.Data
			if p.config.NormalizeWhitespace {
				content = strings.TrimSpace(content)
				if content != "" {
					text.WriteString(content)
					text.WriteString(" ")
				}
			} else {
				text.WriteString(content)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return strings.TrimSpace(text.String())
}

func (p *Parser) extractPlainText(node *html.Node) string {
	var text strings.Builder

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "script" || n.Data == "style" || n.Data == "noscript" {
				return
			}
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

	result := text.String()
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

func (p *Parser) extractMetadata(node *html.Node) map[string]string {
	metadata := make(map[string]string)

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				metadata["title"] = p.getNodeText(n)

			case "meta":
				var name, content, property string
				for _, attr := range n.Attr {
					switch attr.Key {
					case "name":
						name = attr.Val
					case "content":
						content = attr.Val
					case "property":
						property = attr.Val
					}
				}

				if name != "" && content != "" {
					metadata[name] = content
				}
				if property != "" && content != "" {
					metadata[property] = content
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return metadata
}

func (p *Parser) buildNodeTree(node *html.Node, depth int) *HTMLNode {
	if node == nil || depth > p.config.MaxDepth {
		return nil
	}

	htmlNode := &HTMLNode{
		Tag:        node.Data,
		Attributes: make(map[string]string),
		Children:   make([]*HTMLNode, 0),
		Depth:      depth,
	}

	if node.Type == html.ElementNode {
		for _, attr := range node.Attr {
			htmlNode.Attributes[attr.Key] = attr.Val
		}
	}

	if node.Type == html.TextNode {
		htmlNode.Text = strings.TrimSpace(node.Data)
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childNode := p.buildNodeTree(child, depth+1)
		if childNode != nil {
			htmlNode.Children = append(htmlNode.Children, childNode)
		}
	}

	return htmlNode
}

func (p *Parser) validateStructure(node *html.Node) []string {
	errors := make([]string, 0)

	tagStack := make([]string, 0)
	selfClosingTags := map[string]bool{
		"area": true, "base": true, "br": true, "col": true,
		"embed": true, "hr": true, "img": true, "input": true,
		"link": true, "meta": true, "param": true, "source": true,
		"track": true, "wbr": true,
	}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := n.Data

			if !selfClosingTags[tag] {
				tagStack = append(tagStack, tag)
			}

			if tag == "img" {
				hasSrc := false
				for _, attr := range n.Attr {
					if attr.Key == "src" {
						hasSrc = true
						break
					}
				}
				if !hasSrc {
					errors = append(errors, "img tag missing src attribute")
				}
			}

			if tag == "a" {
				hasHref := false
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						hasHref = true
						break
					}
				}
				if !hasHref {
					errors = append(errors, "a tag missing href attribute")
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return errors
}

func (p *Parser) countNodes(node *html.Node) int {
	if node == nil {
		return 0
	}

	count := 1
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		count += p.countNodes(child)
	}

	return count
}

func (p *Parser) calculateDepth(node *html.Node, currentDepth int) int {
	if node == nil {
		return currentDepth
	}

	maxDepth := currentDepth

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		depth := p.calculateDepth(child, currentDepth+1)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

func (p *Parser) hasElement(node *html.Node, tagName string) bool {
	if node == nil {
		return false
	}

	if node.Type == html.ElementNode && node.Data == tagName {
		return true
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if p.hasElement(child, tagName) {
			return true
		}
	}

	return false
}

func (p *Parser) getNodeText(node *html.Node) string {
	var text strings.Builder

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(node)
	return strings.TrimSpace(text.String())
}

func (p *Parser) isExternalLink(url string) bool {
	if url == "" {
		return false
	}

	if strings.HasPrefix(url, "#") || strings.HasPrefix(url, "/") {
		return false
	}

	if strings.HasPrefix(url, "mailto:") || strings.HasPrefix(url, "tel:") {
		return false
	}

	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func (p *Parser) countWords(text string) int {
	if text == "" {
		return 0
	}

	words := strings.Fields(text)
	return len(words)
}

func (p *Parser) SetConfig(config *ParserConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config

	p.log.Info("parser config updated")
}

func (p *Parser) GetConfig() *ParserConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	configCopy := *p.config
	return &configCopy
}

func (p *Parser) ExtractVariables(content string) []string {
	return p.extractVariables(content)
}

func (p *Parser) ExtractURLs(content string) []string {
	matches := p.urlPattern.FindAllString(content, -1)
	return matches
}

func (p *Parser) ExtractEmails(content string) []string {
	matches := p.emailPattern.FindAllString(content, -1)
	return matches
}

func (p *Parser) ValidateHTML(content string) error {
	if content == "" {
		return ErrEmptyContent
	}

	_, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidHTML, err)
	}

	return nil
}

func (p *Parser) GetElementCount(content string, tagName string) (int, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return 0, err
	}

	count := 0
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tagName {
			count++
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(doc)
	return count, nil
}

func (p *Parser) GetAttributeValue(content string, tagName string, attrName string) (string, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return "", err
	}

	var result string
	var traverse func(*html.Node) bool
	traverse = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == tagName {
			for _, attr := range n.Attr {
				if attr.Key == attrName {
					result = attr.Val
					return true
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if traverse(child) {
				return true
			}
		}
		return false
	}

	traverse(doc)
	return result, nil
}

func (p *Parser) StripHTML(content string) string {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return content
	}

	return p.extractPlainText(doc)
}

func (p *Parser) GetTitle(content string) string {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return ""
	}

	var title string
	var traverse func(*html.Node) bool
	traverse = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "title" {
			title = p.getNodeText(n)
			return true
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if traverse(child) {
				return true
			}
		}
		return false
	}

	traverse(doc)
	return title
}

func (p *Parser) HasVariable(content string, varName string) bool {
	variables := p.extractVariables(content)
	for _, v := range variables {
		if v == varName {
			return true
		}
	}
	return false
}

func (p *Parser) GetVariableCount(content string) int {
	return len(p.extractVariables(content))
}

func (p *Parser) ParseBatch(contents []string) ([]*ParseResult, error) {
	results := make([]*ParseResult, len(contents))
	var wg sync.WaitGroup
	errChan := make(chan error, len(contents))

	for i, content := range contents {
		wg.Add(1)
		go func(index int, htmlContent string) {
			defer wg.Done()

			result, err := p.Parse(htmlContent)
			if err != nil {
				errChan <- err
				return
			}

			results[index] = result
		}(i, content)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return results, <-errChan
	}

	return results, nil
}

func (p *Parser) AnalyzeStructure(content string) map[string]interface{} {
	result, err := p.Parse(content)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	return map[string]interface{}{
		"node_count":      result.NodeCount,
		"depth":           result.Depth,
		"word_count":      result.WordCount,
		"char_count":      result.CharCount,
		"link_count":      len(result.Links),
		"image_count":     len(result.Images),
		"variable_count":  len(result.Variables),
		"external_links":  result.ExternalLinks,
		"internal_links":  result.InternalLinks,
		"has_form":        result.HasForm,
		"has_script":      result.HasScript,
		"has_style":       result.HasStyle,
		"errors":          result.Errors,
		"warnings":        result.Warnings,
	}
}

func (p *Parser) GetLinkDensity(content string) float64 {
	result, err := p.Parse(content)
	if err != nil {
		return 0.0
	}

	if result.WordCount == 0 {
		return 0.0
	}

	linkTextCount := 0
	for _, link := range result.Links {
		linkTextCount += len(strings.Fields(link.Text))
	}

	return float64(linkTextCount) / float64(result.WordCount)
}

func (p *Parser) GetImageToTextRatio(content string) float64 {
	result, err := p.Parse(content)
	if err != nil {
		return 0.0
	}

	if result.WordCount == 0 {
		return 0.0
	}

	return float64(len(result.Images)) / float64(result.WordCount)
}
