package utils

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	obfuscationChars    = " .\\-/\\\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−"
	usernamePlaceholder = "клиент"
)

var (
	urlPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)https?://\S+`),
		regexp.MustCompile(`(?i)www\.\S+`),
		regexp.MustCompile(`(?i)tg://\S+`),
		regexp.MustCompile(`(?i)telegram\.me\S*`),
		regexp.MustCompile(`(?i)t\.me/\+\S*`),
		regexp.MustCompile(`(?i)joinchat\S*`),
	}

	obfuscatedDomainPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])[tт][\s\.\-/\\•﹒٫＿_․·∙‧ꞏ‒–—﹘﹣⁻−]*[mм][\s\.\-/\\•﹒٫＿_․·∙‧ꞏ‒–—﹘﹣⁻−]*[eе](?:[^\p{L}\p{N}]|$)`),
		regexp.MustCompile(`(?i)[tт][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*[eе][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*[lłl1i|][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*[eе][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*[gɢgqг][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*[rр][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*[aа][\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]*(?:[mм]|rn)`),
		regexp.MustCompile(`(?i)t\.me\S*`),
	}

	englishServicePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)telegram`),
		regexp.MustCompile(`(?i)teleqram`),
		regexp.MustCompile(`(?i)teiegram`),
		regexp.MustCompile(`(?i)teieqram`),
		regexp.MustCompile(`(?i)telegrarn`),
		regexp.MustCompile(`(?i)service`),
		regexp.MustCompile(`(?i)notif(?:ication)?`),
		regexp.MustCompile(`(?i)system`),
		regexp.MustCompile(`(?i)security`),
		regexp.MustCompile(`(?i)safety`),
		regexp.MustCompile(`(?i)support`),
		regexp.MustCompile(`(?i)moderation`),
		regexp.MustCompile(`(?i)review`),
		regexp.MustCompile(`(?i)compliance`),
		regexp.MustCompile(`(?i)abuse`),
		regexp.MustCompile(`(?i)spam`),
		regexp.MustCompile(`(?i)report`),
	}

	russianServicePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)телеграм\w*`),
		regexp.MustCompile(`(?i)служебн\w*`),
		regexp.MustCompile(`(?i)уведомлен\w*`),
		regexp.MustCompile(`(?i)поддержк\w*`),
		regexp.MustCompile(`(?i)безопасн\w*`),
		regexp.MustCompile(`(?i)модерац\w*`),
		regexp.MustCompile(`(?i)жалоб\w*`),
		regexp.MustCompile(`(?i)абуз\w*`),
	}

	// Ищутся подстрокой в имени, склеенном без пробелов и разделителей:
	// эти слова длинные и в настоящих именах не встречаются.
	normalizedBannedTokens = map[string]bool{
		"telegram":     true,
		"teleqram":     true,
		"teiegram":     true,
		"teieqram":     true,
		"telegrarn":    true,
		"joinchat":     true,
		"notification": true,
		"moderation":   true,
		"compliance":   true,
	}

	// Ищутся только целым словом (можно во множественном числе): подстрокой
	// они цепляют обычные имена — «Абусев», «Reviewer», «Spammer».
	bannedWords = map[string]bool{
		"review": true,
		"abuse":  true,
		"spam":   true,
		"report": true,
	}

	dangerousKeywords = map[string]bool{
		"telegram": true,
		"service":  true,
		"system":   true,
		"security": true,
		"safety":   true,
		"support":  true,
	}

	dangerousCombinations = [][]string{
		{"telegram", "support"},
		{"telegram", "admin"},
		{"service", "support"},
		{"system", "admin"},
		{"security", "admin"},
	}

	preLowerTranslation = map[rune]rune{
		'I': 'l',
		'İ': 'l',
		'Q': 'g',
		'＠': ' ',
	}

	postLowerTranslation = map[rune]string{
		'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d",
		'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
		'і': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m",
		'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s",
		'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c",
		'ч': "ch", 'ш': "sh", 'щ': "sh", 'ъ': "", 'ы': "y",
		'ь': "", 'э': "e", 'ю': "yu", 'я': "ya", '＿': "_",
	}
)

func containsDangerousCombination(normalized string) bool {
	for _, combo := range dangerousCombinations {
		if strings.Contains(normalized, combo[0]) && strings.Contains(normalized, combo[1]) {
			return true
		}
	}
	return false
}

func normalizeForDetection(value string) string {
	if value == "" {
		return ""
	}

	normalized := norm.NFKD.String(value)

	var preLowerBuilder strings.Builder
	for _, r := range normalized {
		if replacement, ok := preLowerTranslation[r]; ok {
			preLowerBuilder.WriteRune(replacement)
		} else {
			preLowerBuilder.WriteRune(r)
		}
	}
	normalized = strings.ToLower(preLowerBuilder.String())

	var builder strings.Builder
	for _, r := range normalized {
		if unicode.In(r, unicode.Mn) {
			continue
		}
		if replacement, ok := postLowerTranslation[r]; ok {
			builder.WriteString(replacement)
		} else {
			builder.WriteRune(r)
		}
	}
	normalized = builder.String()

	normalized = strings.ReplaceAll(normalized, "rn", "m")

	obfuscationRegex := regexp.MustCompile(`[\s\.\-/\\•﹒٫＿․·∙‧ꞏ‒–—﹘﹣⁻−]+`)
	normalized = obfuscationRegex.ReplaceAllString(normalized, "")

	alphanumericRegex := regexp.MustCompile(`[^a-z0-9]+`)
	normalized = alphanumericRegex.ReplaceAllString(normalized, "")

	return normalized
}

// detectionWords разбивает значение на нормализованные слова. Границы слов —
// любые символы кроме букв и цифр, смена регистра (SpamBot → spam, bot)
// и переход между буквами и цифрами (spam123 → spam, 123).
func detectionWords(value string) []string {
	fields := strings.FieldsFunc(stripMarks(norm.NFKD.String(value)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	var words []string
	for _, field := range fields {
		for _, part := range splitCamelAndDigits(field) {
			if normalized := normalizeForDetection(part); normalized != "" {
				words = append(words, normalized)
			}
		}
	}
	return words
}

func stripMarks(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.Mn) {
			return -1
		}
		return r
	}, value)
}

func splitCamelAndDigits(field string) []string {
	runes := []rune(field)
	var parts []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		lowerToUpper := unicode.IsLower(prev) && unicode.IsUpper(cur)
		// Конец аббревиатуры: «TMe» → «T», «Me».
		acronymEnd := unicode.IsUpper(prev) && unicode.IsUpper(cur) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
		letterDigit := unicode.IsDigit(prev) != unicode.IsDigit(cur)
		if lowerToUpper || acronymEnd || letterDigit {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	return append(parts, string(runes[start:]))
}

// containsTelegramLink ищет ссылку t.me только как отдельный токен («t.me», «t me»,
// «t•m•e», «TMe»): несколько подряд идущих слов складываются ровно в «tme».
// Буквы «tme» внутри имени не считаются: иначе банятся «Ахмет Мейрамов», «Just me», «@hotmeal».
func containsTelegramLink(words []string) bool {
	const token = "tme"
	for i := range words {
		joined := ""
		for j := i; j < len(words) && len(joined) < len(token); j++ {
			joined += words[j]
			if joined == token {
				return true
			}
		}
	}
	return false
}

func containsBannedWord(words []string) bool {
	for _, word := range words {
		if bannedWords[word] || bannedWords[strings.TrimSuffix(word, "s")] {
			return true
		}
	}
	return false
}

// containsBannedContent — значение выдаёт себя за Telegram, поддержку или ссылку на t.me.
func containsBannedContent(value string) bool {
	glued := normalizeForDetection(value)
	if glued == "" {
		return false
	}
	for token := range normalizedBannedTokens {
		if strings.Contains(glued, token) {
			return true
		}
	}
	if containsDangerousCombination(glued) {
		return true
	}

	words := detectionWords(value)
	return containsTelegramLink(words) || containsBannedWord(words)
}

func removePatterns(value string) string {
	updated := value
	allPatterns := append([]*regexp.Regexp{}, urlPatterns...)
	allPatterns = append(allPatterns, obfuscatedDomainPatterns...)
	allPatterns = append(allPatterns, englishServicePatterns...)
	allPatterns = append(allPatterns, russianServicePatterns...)

	for _, pattern := range allPatterns {
		updated = pattern.ReplaceAllString(updated, " ")
	}
	return updated
}

func finalize(value string, originalValue string) *string {
	spaceRegex := regexp.MustCompile(`\s+`)
	compacted := spaceRegex.ReplaceAllString(value, " ")
	compacted = strings.Trim(compacted, " \t\r\n-_.,/\\")
	compacted = strings.TrimSpace(compacted)

	if compacted == "" {
		return nil
	}

	if containsBannedContent(originalValue) || containsBannedContent(compacted) {
		return nil
	}

	return &compacted
}

// SanitizeDisplayName cleans and validates display names (first name, last name)
func SanitizeDisplayName(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	original := *value
	clean := strings.ReplaceAll(*value, "@", " ")
	clean = removePatterns(clean)
	return finalize(clean, original)
}

// SanitizeUsername cleans and validates usernames
func SanitizeUsername(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	original := *value
	clean := strings.TrimSpace(*value)
	clean = strings.TrimPrefix(clean, "@")
	clean = removePatterns(clean)
	return finalize(clean, original)
}

// UsernameForDisplay returns a safe display version of username with optional @ prefix
func UsernameForDisplay(username *string, withAt bool) string {
	sanitized := SanitizeUsername(username)
	if sanitized == nil || *sanitized == "" {
		return usernamePlaceholder
	}
	if withAt {
		return "@" + *sanitized
	}
	return *sanitized
}

// DisplayNameOrFallback returns sanitized display name or fallback if empty
func DisplayNameOrFallback(firstName *string, fallback string) string {
	sanitized := SanitizeDisplayName(firstName)
	if sanitized != nil && *sanitized != "" {
		return *sanitized
	}
	if fallback != "" {
		return fallback
	}
	return usernamePlaceholder
}

// IsSuspiciousUser checks if user has suspicious username or display name.
// Поле, целиком состоящее из служебного слова («Support», «System»), само по себе
// не банит: такое имя только скрывается при показе (SanitizeDisplayName).
func IsSuspiciousUser(username *string, firstName *string, lastName *string) bool {
	for _, value := range []*string{username, firstName, lastName} {
		if value != nil && containsBannedContent(*value) {
			return true
		}
	}
	return false
}
