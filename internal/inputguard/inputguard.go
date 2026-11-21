// Package inputguard screens free text typed by customers (questionnaire answers, contact messages)
// before it is stored or handed to an AI model. It rejects:
//
//   - questions about the AI, model, provider or technical setup behind the product,
//   - commands to remove files, data or anything else on the system (shell, SQL, path tricks),
//   - prompt injection ("ignore previous instructions", "you are now an AI…", fake role tags),
//   - malicious payloads (script/HTML injection, SQL injection, template injection),
//   - explicit, hateful or violent content.
//
// It is a deny-list on normalized text, so it is a first line of defense, not the only one: the AI
// prompt also carries fixed safety rules (prompt.SafetyRules) and generated lyrics are checked with
// CheckOutput before they reach the customer.
//
// Patterns are written to avoid blocking real song stories: words that also have an everyday meaning
// ("nembak" = asking someone out, "telanjang kaki", "act as my mom", "you are now my wife") are only
// matched together with an AI/system target.
package inputguard

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Category names why a text was rejected. It is logged, never shown to the customer.
type Category string

const (
	CategoryAIProbe         Category = "ai_probe"         // asks which AI/model/tech is used
	CategoryPromptInjection Category = "prompt_injection" // tries to override the AI's instructions
	CategorySystemCommand   Category = "system_command"   // asks to remove files/data or run commands
	CategoryMaliciousCode   Category = "malicious_code"   // script, SQL or template injection payloads
	CategoryHarmful         Category = "harmful_content"  // explicit, hateful or violent content
)

// Violation is a rejected text: which rule matched.
type Violation struct {
	Category Category
	Match    string // the normalized fragment that matched, for logs only
}

type rule struct {
	cat Category
	re  *regexp.Regexp
}

// aiTarget is what an injection tries to turn the model into.
const aiTarget = `(an? |the )?(ai|a\.i|assistant|asisten|system|sistem|admin|administrator|developer|pengembang|programmer|hacker|chatbot|bot|robot|dan|unrestricted|unfiltered|jailbroken)`

// Patterns run on normalized text (see Normalize) and on folded variants of it.
var patterns = map[Category][]string{
	CategoryAIProbe: {
		// Provider and model names that never belong in a song story. Ambiguous ones that are also
		// people's names or zodiac signs (Claude, Gemini) are only caught in the question patterns below.
		`\b(minimax|openai|chat ?gpt|gpt ?-? ?[0-9]o?|anthropic|deepseek|mistral ai|hugging ?face|suno ai|udio|llama ?[0-9]|bard ai)\b`,
		// Indonesian: "pakai AI apa", "AI apa yang kamu pakai", "teknologi apa yang dipakai"…
		`\b(pakai|pake|memakai|menggunakan|gunakan) (ai|a\.i|model ai|llm|teknologi|tech|api|bahasa pemrograman|framework|server|database|sistem) (apa|mana)\b`,
		`\b(ai|a\.i|model ai|llm|chatbot|bot|teknologi|tech ?stack|api|bahasa pemrograman|framework|server|database|backend|frontend|engine) (apa|mana)( yang| yg)? (kamu|anda|kau|lu|lo|elu|kalian|ini|dipakai|dipake|digunakan|di ?gunakan|dipergunakan|menjalankan|dibalik|di ?balik)\b`,
		`\b(kamu|anda|kau|lu|lo|elu|ini) (itu )?(ai|a\.i|bot|robot|chatbot|llm) (apa|siapa|buatan siapa)\b`,
		`\b(siapa|apa) (yang |yg )?(membuat|bikin|pembuat|developer|pengembang|programmer|provider|penyedia|vendor) (kamu|anda|ai|model ai|sistem|aplikasi|app|website|situs) ?(ini)?\b`,
		`\b(sebutkan|beritahu|beri tahu|kasih tahu|kasih tau|kasitau|jelaskan|tunjukkan|tampilkan|bocorkan) .{0,30}\b(model ai|llm|teknologi|tech ?stack|api ?key|kunci api|prompt|instruksi sistem|sistem ai|server|database|source ?code|kode sumber|konfigurasi|config|provider ai|versi model)\b`,
		`\b(versi|nama) (model ai|llm|prompt|server|database|api)\b`,
		// English.
		`\bwhat (ai|a\.i|model|llm|language model|tech|technology|stack|tech stack|api|engine|backend|framework|database|server|provider)\b.{0,20}\b(are|do|is|does|did) (you|this|it)\b`,
		`\bwhich (ai|a\.i|llm|language model|ai model|engine|ai provider|api)\b`,
		`\b(are you|is this|r u) (an? )?(ai|a\.i|bot|chatbot|gpt|llm|language model|claude|gemini)\b`,
		`\bwho (made|built|created|trained|developed|programmed|owns) (you|this ai|this app|the ai|the model)\b`,
		`\b(tell|show|reveal|give|print|list|describe|explain)( me)? .{0,30}\b(ai model|llm|tech ?stack|api ?key|system prompt|your prompt|your instructions|source ?code|configuration|env(ironment)? var\w*|ai provider)\b`,
		`\b(tech ?stack|system prompt|prompt sistem|api ?key|secret key|access token|source code|kode sumber|environment variable)\b`,
	},
	CategoryPromptInjection: {
		`\b(ignore|disregard|forget|override|bypass)( all| any| the| your)? (previous|prior|above|earlier|preceding|system|original|initial) (instructions?|prompts?|rules?|directions?|guidelines?|context)\b`,
		`\b(ignore|disregard|override|bypass) (all |any |your |the )?(instructions?|prompts?|system prompt|guidelines)\b`,
		`\b(abaikan|lupakan|acuhkan|hiraukan|timpa)( semua| seluruh| segala)? (instruksi|perintah|aturan|arahan|prompt|pedoman|batasan|konteks)( sebelumnya| di atas| sistem| ai)?\b`,
		`\b(you are now|from now on,? you are|you will now be|you must now be|act as|pretend to be|pretend you are|role ?play as|you are no longer) ` + aiTarget + `\b`,
		`\b(kamu|anda|kau|lu|elu) (sekarang|mulai sekarang) (adalah|jadi|menjadi|bertindak sebagai|berperan sebagai) ` + aiTarget + `\b`,
		`\b(berperan|bertindak|berpura ?-? ?pura) (sebagai|jadi|menjadi) ` + aiTarget + `\b`,
		`\b(new instructions?|instruksi baru|developer mode|mode developer|mode pengembang|dan mode|do anything now|jailbreak\w*)\b`,
		`\b(instead of (writing )?(a |the )?(song|lyrics)|don'?t write (a |the )?(song|lyrics)|jangan (tulis|menulis|buat) lirik,)\b`,
		// Chat-template and role markers.
		`(<\|?(im_start|im_end|system|endoftext|assistant)\|?>|\[/?(system|inst|assistant)\]|#{2,} ?(system|instruction|assistant)|\b(system|assistant) ?:)`,
	},
	CategorySystemCommand: {
		// Shell commands followed by a flag, path, wildcard or URL.
		`\b(rm|rmdir|shred|mkfs|chmod|chown|sudo|killall|pkill|shutdown|reboot|poweroff|wget|curl|netcat|ncat|bash|zsh|powershell|crontab|ssh|scp)\b ?(-[a-z]+|/|~|\*|[a-z]:\\|https?:|if=)`,
		`\b(rm ?-? ?rf|rmrf)\b`,
		`(:\(\)\{|\$\(|&&|\|\||;\s*(rm|del|drop|shutdown|curl|wget)\b|>\s*/dev/)`,
		`(/etc/(passwd|shadow|hosts)|/var/|/usr/|/bin/|/root\b|~/\.|c:\\|%systemroot%|system32|\.\./|\.\.\\|\.ssh\b|id_rsa|\.bashrc|\.git/|\.env\b)`,
		// SQL statements that change or remove schema/data.
		`\b(drop|truncate|alter) (table|database|schema|user|index)\b`,
		`\b(delete|truncate)\s+(\*\s+)?(from|table)\b`,
		`\b(insert into|update [a-z_]+ set|grant all|exec(ute)? (xp_|sp_))\b`,
		// Code calls.
		`(\bos\.(remove|system|unlink|rmdir)|\bsubprocess\b|shutil\.rmtree|child_process|process\.exit|runtime\.exec|\bsystem\(|\bexec\(|\beval\(|\bpopen\b|\bunlink\()`,
		// Natural-language requests, Indonesian ("hapus semua file", "retas server", "kosongkan database").
		`\b(hapus|menghapus|hapuskan|dihapus|delete|buang|membuang|musnahkan|kosongkan|format|wipe|reset|rusak|merusak|matikan|mematikan|retas|meretas|hack|bobol|membobol|mengakses|unduh|curi|mencuri|bocorkan)( semua| seluruh| isi)? (file|berkas|data|database|db|basis data|tabel|folder|direktori|sistem|system|server|aplikasi|website|situs|akun admin|admin|config|konfigurasi|log|backup|storage|penyimpanan|source code|password|kata sandi|token|user|pengguna)\b`,
		// English.
		`\b(delete|remove|erase|wipe|destroy|drop|purge|format|corrupt|hack|crash|shut ?down|take down|dump|exfiltrate|leak|steal)( all| the| every| your| this)? (files?|folders?|director(y|ies)|data|database|db|tables?|system|server|app|application|website|site|config|logs?|backups?|storage|source code|passwords?|tokens?|users?|admin|records?)\b`,
	},
	CategoryMaliciousCode: {
		`(<\s*/?\s*(script|iframe|object|embed|svg|img|link|meta|style|form|input|body|html|base|frame|applet)\b|javascript\s*:|vbscript\s*:|data\s*:\s*text/html|\bon(error|load|click|mouse\w*|focus|blur|submit|change|key\w*)\s*=)`,
		`(('|")\s*(or|and)\s*('|")?[0-9a-z]+('|")?\s*=|\bor\s+1\s*=\s*1\b|\bunion(\s+all)?\s+select\b|\bsleep\s*\(\s*[0-9]|\bbenchmark\s*\(|\bwaitfor delay\b|;\s*--|/\*.*\*/)`,
		`(\$\{|\{\{|\}\}|\{%|%\}|<%|%>|<\?php|<\?=|#\{)`,
		`(\\x[0-9a-f]{2}|\\u[0-9a-f]{4}|%[0-9a-f]{2}%[0-9a-f]{2}|&#x?[0-9a-f]+;)`,
	},
	CategoryHarmful: {
		// Indonesian vulgar, sexual and hateful terms (whole words only; "anjing"/"babi" on their own are
		// left out because pets and nicknames appear in real stories).
		`\b(ngentot|ngewe|entot|kontol|memek|pepek|jembut|toket|colmek|bokep|porno|pornografi|bugil|ngocok|sange|lonte|pelacur|jancok|jancuk|bangsat|bajingan|keparat|goblok|tolol|anjing lu|babi lu|anjing kau|babi kau)\b`,
		`\b(bunuh|membunuh|bantai|membantai|perkosa|memperkosa|siksa|menyiksa|ledakkan|meledakkan|racuni|meracuni)( saja| aja)? (kamu|dia|mereka|kalian|lu|elu|dirimu|anak)\b`,
		`\b(bunuh diri|gantung diri|bom bunuh diri|cara membuat bom|merakit bom|teroris|terorisme|jual narkoba)\b`,
		// English.
		`\b(fuck\w*|motherfuck\w*|bitch\w*|cunt|porn\w*|nudes|rape\w*|nigg\w*|faggot\w*|retard\w*|whore|slut)\b`,
		`\b(kill|murder|stab|bomb|rape|kidnap|torture|poison)( him| her| them| you| everyone| people| kids)\b`,
		`\b(kill yourself|kys|how to make a bomb|terrorist|terrorism|white power|heil hitler)\b`,
	},
}

// order decides which category is reported when several match.
var order = []Category{CategoryPromptInjection, CategorySystemCommand, CategoryMaliciousCode, CategoryAIProbe, CategoryHarmful}

var rules = func() []rule {
	var out []rule
	for _, cat := range order {
		for _, p := range patterns[cat] {
			out = append(out, rule{cat: cat, re: regexp.MustCompile(`(?is)` + p)})
		}
	}
	return out
}()

var (
	// controlChars are invisible bytes that have no place in a story and are used to smuggle payloads.
	controlChars = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	zeroWidth    = regexp.MustCompile("[\u200b-\u200f\u2060-\u2064\ufeff\u00ad]")
	spaceRe      = regexp.MustCompile(`\s+`)
	leetFolder   = strings.NewReplacer("0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t", "@", "a", "$", "s")
	// spelledOut is three or more single characters separated by spaces/dots/dashes: "m i n i m a x".
	spelledOut = regexp.MustCompile(`\b[a-z](?:[ .\-_*]+[a-z]\b){2,}`)
	// innerSep is punctuation inside a word: "mini.max", "kon-tol", "drop_table".
	innerSep = regexp.MustCompile(`([a-z])[\-_.*]+([a-z])`)
	nonAlpha = regexp.MustCompile(`[^a-z]+`)
)

// variants are the folded copies of normalized text the rules run on.
func variants(n string) []string {
	leet := leetFolder.Replace(n)
	joined := spelledOut.ReplaceAllStringFunc(leet, func(s string) string { return nonAlpha.ReplaceAllString(s, "") })
	unsep := innerSep.ReplaceAllString(leet, "$1$2")
	return []string{n, leet, joined, unsep}
}

// Check returns the first rule text breaks, or ok=false when the text is clean.
func Check(text string) (Violation, bool) {
	if strings.TrimSpace(text) == "" {
		return Violation{}, false
	}
	if controlChars.MatchString(text) {
		return Violation{Category: CategoryMaliciousCode, Match: "control character"}, true
	}
	for _, s := range variants(Normalize(text)) {
		for _, r := range rules {
			if m := r.re.FindString(s); m != "" {
				return Violation{Category: r.cat, Match: m}, true
			}
		}
	}
	return Violation{}, false
}

// Normalize folds text for matching: full-width and accented look-alikes to plain letters,
// zero-width characters removed, lower case, whitespace collapsed to single spaces.
func Normalize(s string) string {
	t := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFKC)
	if out, _, err := transform.String(t, s); err == nil {
		s = out
	}
	s = zeroWidth.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// leakRe matches technical details that must never appear in AI output shown to customers.
var leakRe = regexp.MustCompile(`(?i)\b(minimax|openai|chat ?gpt|gpt ?-? ?[0-9]o?|anthropic|claude ai|gemini ai|deepseek|large language model|language model|model bahasa|kecerdasan buatan|artificial intelligence|system prompt|prompt sistem|api ?key|as an ai|sebagai ai|saya adalah ai|aku adalah ai|i am an ai|i'?m an ai)\b`)

// CheckOutput reports AI-generated text (title, lyrics) that leaks technical details (provider,
// model, prompt) or carries harmful or code-like content. The pipeline treats a hit as an invalid
// output and retries the call.
func CheckOutput(text string) (Violation, bool) {
	n := Normalize(text)
	if m := leakRe.FindString(n); m != "" {
		return Violation{Category: CategoryAIProbe, Match: m}, true
	}
	for _, r := range rules {
		if r.cat != CategoryHarmful && r.cat != CategoryMaliciousCode {
			continue
		}
		if m := r.re.FindString(n); m != "" {
			return Violation{Category: r.cat, Match: m}, true
		}
	}
	return Violation{}, false
}
