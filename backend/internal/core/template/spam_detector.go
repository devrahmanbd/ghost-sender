package template

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"email-campaign-system/pkg/logger"
)

type SpamDetector struct {
	log            logger.Logger
	config         *SpamDetectorConfig
	spamWords      map[string]float64
	spamPhrases    map[string]float64
	suspiciousURLs []string
	mu             sync.RWMutex
}

type SpamDetectorConfig struct {
	MaxSpamScore        float64
	WarningThreshold    float64
	CriticalThreshold   float64
	MaxLinkDensity      float64
	MaxCapitalization   float64
	MaxPunctuation      float64
	MinTextLength       int
	MaxExclamationMarks int
	MaxQuestionMarks    int
	CheckSubject        bool
	CheckBody           bool
	CheckLinks          bool
	CheckImages         bool
	CheckFormatting     bool
	StrictMode          bool
}

type SpamAnalysisResult struct {
	Score              float64
	Grade              string
	Risk               string
	Passed             bool
	Indicators         []SpamIndicator
	Recommendations    []string
	ContentAnalysis    *ContentAnalysis
	SubjectAnalysis    *SubjectAnalysis
	LinkAnalysis       *LinkAnalysis
	FormattingAnalysis *FormattingAnalysis
	Statistics         *SpamStatistics
}

type SpamIndicator struct {
	Type        string
	Severity    string
	Description string
	Impact      float64
	Location    string
}

type ContentAnalysis struct {
	SpamWordCount       int
	SpamPhraseCount     int
	SuspiciousPatterns  int
	CapitalizationRatio float64
	PunctuationRatio    float64
	TextLength          int
	UniqueWords         int
}

type SubjectAnalysis struct {
	Length               int
	HasSpamWords         bool
	ExcessiveCaps        bool
	ExcessivePunctuation bool
	Score                float64
	Issues               []string
}

type LinkAnalysis struct {
	TotalLinks      int
	ExternalLinks   int
	SuspiciousLinks int
	ShortenedLinks  int
	LinkDensity     float64
	Score           float64
}

type FormattingAnalysis struct {
	ExcessiveBold    bool
	ExcessiveColors  bool
	ExcessiveFonts   bool
	ExcessiveImages  bool
	ImageToTextRatio float64
	Score            float64
}

type SpamStatistics struct {
	TotalChecks    int
	SpamDetected   int
	FalsePositives int
	AverageScore   float64
	HighestScore   float64
	LowestScore    float64
}

func NewSpamDetector(log logger.Logger) *SpamDetector {
	config := DefaultSpamDetectorConfig()

	detector := &SpamDetector{
		log:            log,
		config:         config,
		spamWords:      make(map[string]float64),
		spamPhrases:    make(map[string]float64),
		suspiciousURLs: []string{},
	}

	detector.initializeSpamDatabase()

	return detector
}

func DefaultSpamDetectorConfig() *SpamDetectorConfig {
	return &SpamDetectorConfig{
		MaxSpamScore:        10.0,
		WarningThreshold:    5.0,
		CriticalThreshold:   7.0,
		MaxLinkDensity:      0.3,
		MaxCapitalization:   0.3,
		MaxPunctuation:      0.1,
		MinTextLength:       50,
		MaxExclamationMarks: 3,
		MaxQuestionMarks:    2,
		CheckSubject:        true,
		CheckBody:           true,
		CheckLinks:          true,
		CheckImages:         true,
		CheckFormatting:     true,
		StrictMode:          false,
	}
}

func (sd *SpamDetector) initializeSpamDatabase() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.spamWords = map[string]float64{
		"free": 1.5, "click": 1.0, "buy": 1.2, "win": 1.5, "winner": 1.5,
		"cash": 1.3, "money": 1.2, "prize": 1.4, "bonus": 1.3, "gift": 1.2,
		"deal": 1.0, "offer": 1.0, "discount": 1.1, "cheap": 1.2, "affordable": 0.8,
		"guarantee": 1.2, "risk-free": 1.3, "no obligation": 1.3, "limited time": 1.1,
		"act now": 1.4, "urgent": 1.3, "hurry": 1.2, "expires": 1.1, "today": 0.8,
		"credit": 1.2, "loan": 1.3, "debt": 1.2, "mortgage": 1.1, "refinance": 1.1,
		"viagra": 2.0, "cialis": 2.0, "pharmacy": 1.5, "pills": 1.5, "medication": 1.2,
		"weight loss": 1.6, "lose weight": 1.6, "diet": 1.1, "fat": 1.0,
		"million": 1.4, "billion": 1.5, "dollars": 1.2, "income": 1.1, "earn": 1.2,
		"investment": 1.1, "profit": 1.2, "revenue": 1.0, "mlm": 1.8, "network marketing": 1.7,
		"congratulations": 1.3, "selected": 1.3, "chosen": 1.3, "lucky": 1.4,
		"unsubscribe": 0.5, "opt-out": 0.5, "remove": 0.7, "stop": 0.6,
		"guaranteed": 1.3, "certified": 0.9, "approved": 1.0, "verified": 0.9,
		"explode": 1.2, "amazing": 1.1, "incredible": 1.2, "revolutionary": 1.3,
	}

	sd.spamPhrases = map[string]float64{
		"click here": 1.5, "click now": 1.6, "click below": 1.4,
		"limited time offer": 1.5, "act now": 1.6, "don't wait": 1.4,
		"buy now": 1.5, "order now": 1.5, "call now": 1.4,
		"free money": 2.0, "make money": 1.7, "earn money": 1.6,
		"risk free": 1.5, "no risk": 1.5, "100% free": 1.8,
		"dear friend": 1.3, "dear sir": 1.2, "hello dear": 1.3,
		"nigerian prince": 2.5, "bank transfer": 1.6, "wire transfer": 1.5,
		"inheritance": 1.8, "lottery": 1.9, "sweepstakes": 1.7,
		"weight loss": 1.6, "lose weight fast": 1.8, "burn fat": 1.6,
		"work from home": 1.5, "make money online": 1.7, "passive income": 1.4,
	}

	sd.suspiciousURLs = []string{
		"bit.ly", "tinyurl.com", "goo.gl", "ow.ly", "t.co",
		"shorturl.at", "is.gd", "buff.ly",
	}
}

func (sd *SpamDetector) Analyze(subject, body string) (*SpamAnalysisResult, error) {
	result := &SpamAnalysisResult{
		Indicators:         make([]SpamIndicator, 0),
		Recommendations:    make([]string, 0),
		ContentAnalysis:    &ContentAnalysis{},
		SubjectAnalysis:    &SubjectAnalysis{},
		LinkAnalysis:       &LinkAnalysis{},
		FormattingAnalysis: &FormattingAnalysis{},
		Statistics:         &SpamStatistics{},
	}

	if sd.config.CheckSubject {
		sd.analyzeSubject(subject, result)
	}

	if sd.config.CheckBody {
		sd.analyzeContent(body, result)
	}

	if sd.config.CheckLinks {
		sd.analyzeLinks(body, result)
	}

	if sd.config.CheckImages {
		sd.analyzeImages(body, result)
	}

	if sd.config.CheckFormatting {
		sd.analyzeFormatting(body, result)
	}

	sd.calculateFinalScore(result)
	sd.generateRecommendations(result)

	return result, nil
}

func (sd *SpamDetector) analyzeSubject(subject string, result *SpamAnalysisResult) {
	result.SubjectAnalysis.Length = len(subject)

	if len(subject) == 0 {
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "empty_subject",
			Severity:    "critical",
			Description: "Subject line is empty",
			Impact:      2.0,
			Location:    "subject",
		})
		return
	}

	capsCount := 0
	for _, ch := range subject {
		if ch >= 'A' && ch <= 'Z' {
			capsCount++
		}
	}
	capsRatio := float64(capsCount) / float64(len(subject))

	if capsRatio > sd.config.MaxCapitalization {
		result.SubjectAnalysis.ExcessiveCaps = true
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "excessive_caps",
			Severity:    "warning",
			Description: fmt.Sprintf("Subject has %.1f%% capital letters", capsRatio*100),
			Impact:      1.5,
			Location:    "subject",
		})
	}

	exclamationCount := strings.Count(subject, "!")
	if exclamationCount > sd.config.MaxExclamationMarks {
		result.SubjectAnalysis.ExcessivePunctuation = true
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "excessive_exclamation",
			Severity:    "warning",
			Description: fmt.Sprintf("Subject has %d exclamation marks", exclamationCount),
			Impact:      1.2,
			Location:    "subject",
		})
	}

	subjectLower := strings.ToLower(subject)
	for word, weight := range sd.spamWords {
		if strings.Contains(subjectLower, word) {
			result.SubjectAnalysis.HasSpamWords = true
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "spam_word_subject",
				Severity:    "warning",
				Description: fmt.Sprintf("Spam word '%s' found in subject", word),
				Impact:      weight,
				Location:    "subject",
			})
		}
	}

	for phrase, weight := range sd.spamPhrases {
		if strings.Contains(subjectLower, phrase) {
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "spam_phrase_subject",
				Severity:    "critical",
				Description: fmt.Sprintf("Spam phrase '%s' found in subject", phrase),
				Impact:      weight * 1.5,
				Location:    "subject",
			})
		}
	}
}

func (sd *SpamDetector) analyzeContent(body string, result *SpamAnalysisResult) {
	plainText := sd.stripHTML(body)
	result.ContentAnalysis.TextLength = len(plainText)

	if len(plainText) < sd.config.MinTextLength {
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "short_content",
			Severity:    "info",
			Description: fmt.Sprintf("Content is too short (%d characters)", len(plainText)),
			Impact:      0.5,
			Location:    "body",
		})
	}

	bodyLower := strings.ToLower(plainText)
	spamWordCount := 0
	for word, weight := range sd.spamWords {
		count := strings.Count(bodyLower, word)
		if count > 0 {
			spamWordCount += count
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "spam_word_body",
				Severity:    "warning",
				Description: fmt.Sprintf("Spam word '%s' appears %d times", word, count),
				Impact:      weight * float64(count) * 0.5,
				Location:    "body",
			})
		}
	}
	result.ContentAnalysis.SpamWordCount = spamWordCount

	spamPhraseCount := 0
	for phrase, weight := range sd.spamPhrases {
		count := strings.Count(bodyLower, phrase)
		if count > 0 {
			spamPhraseCount += count
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "spam_phrase_body",
				Severity:    "critical",
				Description: fmt.Sprintf("Spam phrase '%s' appears %d times", phrase, count),
				Impact:      weight * float64(count),
				Location:    "body",
			})
		}
	}
	result.ContentAnalysis.SpamPhraseCount = spamPhraseCount

	capsCount := 0
	letterCount := 0
	for _, ch := range plainText {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			letterCount++
			if ch >= 'A' && ch <= 'Z' {
				capsCount++
			}
		}
	}

	if letterCount > 0 {
		capsRatio := float64(capsCount) / float64(letterCount)
		result.ContentAnalysis.CapitalizationRatio = capsRatio

		if capsRatio > sd.config.MaxCapitalization {
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "excessive_caps_body",
				Severity:    "warning",
				Description: fmt.Sprintf("Body has %.1f%% capital letters", capsRatio*100),
				Impact:      1.0,
				Location:    "body",
			})
		}
	}

	punctuationCount := 0
	punctuationChars := "!?.,;:"
	for _, ch := range plainText {
		if strings.ContainsRune(punctuationChars, ch) {
			punctuationCount++
		}
	}

	if len(plainText) > 0 {
		punctRatio := float64(punctuationCount) / float64(len(plainText))
		result.ContentAnalysis.PunctuationRatio = punctRatio

		if punctRatio > sd.config.MaxPunctuation {
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "excessive_punctuation",
				Severity:    "warning",
				Description: fmt.Sprintf("Body has %.1f%% punctuation", punctRatio*100),
				Impact:      0.8,
				Location:    "body",
			})
		}
	}
}

func (sd *SpamDetector) analyzeLinks(body string, result *SpamAnalysisResult) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return
	}

	var links []string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					links = append(links, attr.Val)
					break
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}
	traverse(doc)

	result.LinkAnalysis.TotalLinks = len(links)

	externalCount := 0
	suspiciousCount := 0
	shortenedCount := 0

	for _, link := range links {
		linkLower := strings.ToLower(link)

		if strings.HasPrefix(linkLower, "http://") || strings.HasPrefix(linkLower, "https://") {
			externalCount++

			for _, suspiciousDomain := range sd.suspiciousURLs {
				if strings.Contains(linkLower, suspiciousDomain) {
					suspiciousCount++
					result.Indicators = append(result.Indicators, SpamIndicator{
						Type:        "suspicious_link",
						Severity:    "critical",
						Description: fmt.Sprintf("Suspicious URL shortener detected: %s", link),
						Impact:      1.5,
						Location:    "body",
					})
					break
				}
			}

			if len(link) < 30 && strings.Count(link, "/") > 2 {
				shortenedCount++
			}
		}

		if strings.Contains(linkLower, "unsubscribe") || strings.Contains(linkLower, "opt-out") {
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "unsubscribe_link",
				Severity:    "info",
				Description: "Unsubscribe link found (good practice)",
				Impact:      -0.5,
				Location:    "body",
			})
		}
	}

	result.LinkAnalysis.ExternalLinks = externalCount
	result.LinkAnalysis.SuspiciousLinks = suspiciousCount
	result.LinkAnalysis.ShortenedLinks = shortenedCount

	plainText := sd.stripHTML(body)
	wordCount := len(strings.Fields(plainText))
	if wordCount > 0 {
		linkDensity := float64(len(links)) / float64(wordCount)
		result.LinkAnalysis.LinkDensity = linkDensity

		if linkDensity > sd.config.MaxLinkDensity {
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "high_link_density",
				Severity:    "warning",
				Description: fmt.Sprintf("Link density %.2f exceeds threshold %.2f", linkDensity, sd.config.MaxLinkDensity),
				Impact:      1.2,
				Location:    "body",
			})
		}
	}
}

func (sd *SpamDetector) analyzeImages(body string, result *SpamAnalysisResult) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return
	}

	imageCount := 0
	imagesWithoutAlt := 0

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			imageCount++

			hasAlt := false
			for _, attr := range n.Attr {
				if attr.Key == "alt" && attr.Val != "" {
					hasAlt = true
					break
				}
			}

			if !hasAlt {
				imagesWithoutAlt++
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}
	traverse(doc)

	if imagesWithoutAlt > 0 {
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "missing_alt_tags",
			Severity:    "info",
			Description: fmt.Sprintf("%d images without alt tags", imagesWithoutAlt),
			Impact:      0.3,
			Location:    "body",
		})
	}

	plainText := sd.stripHTML(body)
	textLength := len(strings.TrimSpace(plainText))

	if textLength > 0 && imageCount > 0 {
		imageToTextRatio := float64(imageCount*1000) / float64(textLength)
		result.FormattingAnalysis.ImageToTextRatio = imageToTextRatio

		if imageToTextRatio > 5.0 {
			result.FormattingAnalysis.ExcessiveImages = true
			result.Indicators = append(result.Indicators, SpamIndicator{
				Type:        "high_image_ratio",
				Severity:    "warning",
				Description: "Too many images relative to text content",
				Impact:      1.0,
				Location:    "body",
			})
		}
	}
}

func (sd *SpamDetector) analyzeFormatting(body string, result *SpamAnalysisResult) {
	boldCount := strings.Count(strings.ToLower(body), "<b>") + strings.Count(strings.ToLower(body), "<strong>")
	if boldCount > 10 {
		result.FormattingAnalysis.ExcessiveBold = true
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "excessive_bold",
			Severity:    "info",
			Description: fmt.Sprintf("Too many bold tags (%d)", boldCount),
			Impact:      0.5,
			Location:    "body",
		})
	}

	colorPattern := regexp.MustCompile(`color\s*:\s*#?[0-9a-fA-F]{3,6}`)
	colorMatches := colorPattern.FindAllString(body, -1)
	uniqueColors := make(map[string]bool)
	for _, match := range colorMatches {
		uniqueColors[match] = true
	}

	if len(uniqueColors) > 5 {
		result.FormattingAnalysis.ExcessiveColors = true
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "excessive_colors",
			Severity:    "info",
			Description: fmt.Sprintf("Too many different colors (%d)", len(uniqueColors)),
			Impact:      0.4,
			Location:    "body",
		})
	}

	fontPattern := regexp.MustCompile(`font-family\s*:\s*[^;]+`)
	fontMatches := fontPattern.FindAllString(body, -1)
	uniqueFonts := make(map[string]bool)
	for _, match := range fontMatches {
		uniqueFonts[match] = true
	}

	if len(uniqueFonts) > 3 {
		result.FormattingAnalysis.ExcessiveFonts = true
		result.Indicators = append(result.Indicators, SpamIndicator{
			Type:        "excessive_fonts",
			Severity:    "info",
			Description: fmt.Sprintf("Too many different fonts (%d)", len(uniqueFonts)),
			Impact:      0.3,
			Location:    "body",
		})
	}
}

func (sd *SpamDetector) calculateFinalScore(result *SpamAnalysisResult) {
	totalScore := 0.0

	for _, indicator := range result.Indicators {
		totalScore += indicator.Impact
	}

	if totalScore < 0 {
		totalScore = 0
	}

	if totalScore > sd.config.MaxSpamScore {
		totalScore = sd.config.MaxSpamScore
	}

	result.Score = totalScore

	if totalScore <= 2.0 {
		result.Grade = "A"
		result.Risk = "low"
		result.Passed = true
	} else if totalScore <= 4.0 {
		result.Grade = "B"
		result.Risk = "low"
		result.Passed = true
	} else if totalScore <= 6.0 {
		result.Grade = "C"
		result.Risk = "medium"
		result.Passed = true
	} else if totalScore <= 8.0 {
		result.Grade = "D"
		result.Risk = "high"
		result.Passed = false
	} else {
		result.Grade = "F"
		result.Risk = "critical"
		result.Passed = false
	}
}

func (sd *SpamDetector) generateRecommendations(result *SpamAnalysisResult) {
	if result.SubjectAnalysis.HasSpamWords {
		result.Recommendations = append(result.Recommendations, "Remove spam trigger words from subject line")
	}

	if result.SubjectAnalysis.ExcessiveCaps {
		result.Recommendations = append(result.Recommendations, "Reduce capitalization in subject line")
	}

	if result.SubjectAnalysis.ExcessivePunctuation {
		result.Recommendations = append(result.Recommendations, "Reduce excessive punctuation marks in subject")
	}

	if result.ContentAnalysis.SpamWordCount > 5 {
		result.Recommendations = append(result.Recommendations, "Reduce spam trigger words in email body")
	}

	if result.ContentAnalysis.SpamPhraseCount > 0 {
		result.Recommendations = append(result.Recommendations, "Remove spam phrases from email content")
	}

	if result.ContentAnalysis.CapitalizationRatio > sd.config.MaxCapitalization {
		result.Recommendations = append(result.Recommendations, "Use less capitalization in email body")
	}

	if result.LinkAnalysis.SuspiciousLinks > 0 {
		result.Recommendations = append(result.Recommendations, "Avoid using URL shorteners")
	}

	if result.LinkAnalysis.LinkDensity > sd.config.MaxLinkDensity {
		result.Recommendations = append(result.Recommendations, "Reduce the number of links in email")
	}

	if result.FormattingAnalysis.ExcessiveBold {
		result.Recommendations = append(result.Recommendations, "Use bold formatting more sparingly")
	}

	if result.FormattingAnalysis.ExcessiveColors {
		result.Recommendations = append(result.Recommendations, "Limit the number of different colors used")
	}

	if result.FormattingAnalysis.ExcessiveImages {
		result.Recommendations = append(result.Recommendations, "Balance images with text content")
	}

	if result.ContentAnalysis.TextLength < sd.config.MinTextLength {
		result.Recommendations = append(result.Recommendations, "Add more meaningful text content")
	}

	hasUnsubscribe := false
	for _, indicator := range result.Indicators {
		if indicator.Type == "unsubscribe_link" {
			hasUnsubscribe = true
			break
		}
	}
	if !hasUnsubscribe {
		result.Recommendations = append(result.Recommendations, "Add an unsubscribe link")
	}
}

func (sd *SpamDetector) stripHTML(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

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

	traverse(doc)
	return strings.TrimSpace(text.String())
}

func (sd *SpamDetector) SetConfig(config *SpamDetectorConfig) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.config = config

	sd.log.Info("spam detector config updated")
}

func (sd *SpamDetector) GetConfig() *SpamDetectorConfig {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	configCopy := *sd.config
	return &configCopy
}

func (sd *SpamDetector) AddSpamWord(word string, weight float64) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.spamWords[strings.ToLower(word)] = weight
}

func (sd *SpamDetector) RemoveSpamWord(word string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	delete(sd.spamWords, strings.ToLower(word))
}

func (sd *SpamDetector) AddSpamPhrase(phrase string, weight float64) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.spamPhrases[strings.ToLower(phrase)] = weight
}

func (sd *SpamDetector) RemoveSpamPhrase(phrase string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	delete(sd.spamPhrases, strings.ToLower(phrase))
}

func (sd *SpamDetector) QuickCheck(subject, body string) float64 {
	result, err := sd.Analyze(subject, body)
	if err != nil {
		return sd.config.MaxSpamScore
	}

	return result.Score
}

func (sd *SpamDetector) IsSpam(subject, body string) bool {
	score := sd.QuickCheck(subject, body)
	return score >= sd.config.CriticalThreshold
}

func (sd *SpamDetector) GetRisk(subject, body string) string {
	result, err := sd.Analyze(subject, body)
	if err != nil {
		return "unknown"
	}

	return result.Risk
}

func (sd *SpamDetector) GetGrade(subject, body string) string {
	result, err := sd.Analyze(subject, body)
	if err != nil {
		return "F"
	}

	return result.Grade
}

func (sd *SpamDetector) ExportSpamDatabase() map[string]interface{} {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	words := make(map[string]float64)
	for k, v := range sd.spamWords {
		words[k] = v
	}

	phrases := make(map[string]float64)
	for k, v := range sd.spamPhrases {
		phrases[k] = v
	}

	return map[string]interface{}{
		"spam_words":      words,
		"spam_phrases":    phrases,
		"suspicious_urls": sd.suspiciousURLs,
	}
}

func (sd *SpamDetector) GetSpamWordCount() int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	return len(sd.spamWords)
}

func (sd *SpamDetector) GetSpamPhraseCount() int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	return len(sd.spamPhrases)
}
