package security

import (
	"strings"
	"testing"
)

// The values below are assembled at run time, so this file holds no
// string that a credential scanner would report.
var newSamples = map[string]string{
	"github fine grained token":  j("github_pat_", "11ABCDEFG0", strings.Repeat("aB3dE", 12)),
	"gitlab token":               j("glpat-", "aB3dE", "xY7zQ9wM2nK4pL6r"),
	"google oauth client secret": j("GOCSPX-", "aB3dE7zQ9wM2nK4pL6rT8v"),
	"hugging face token":         j("hf_", strings.Repeat("aBcDe", 7)),
	"sendgrid api key":           j("SG.", "aB3dE7zQ9wM2nK4p", ".", "L6rT8vX1yU5iO0sD9fG"),
	"twilio api key":             j("SK", "0123456789abcdef0123456789abcdef"),
	"shopify access token":       j("shpat_", "0123456789abcdef0123456789abcdef"),
	"square access token":        j("sq0atp-", "aB3dE7zQ9wM2nK4pL6rT8v"),
	"mailgun api key":            j("key-", "0123456789abcdef0123456789abcdef"),
	"new relic key":              j("NRAK-", "ABCDEFGHIJKLMNOPQRSTU"),
	"telegram bot token":         j("123456789", ":AA", "Fk3lPq7sT9vX2zB5nM8cV1rY4uI6oE0wQ"),
	"slack webhook":              j("https://hooks.slack.com/services/", "T00000000/B00000000/", "aB3dE7zQ9wM2nK4pL6rT8vX1"),
	"azure storage key":          j("AccountKey=", strings.Repeat("aB3dE7zQ9w", 7), "=="),
	"putty private key":          j("PuTTY-User-Key-File-3", ": ssh-rsa"),
	"google service account key": j(`"type"`, `: `, `"service_account"`),
	"basic authorization header": j("Authorization: ", "Basic ", "dXNlcjpodW50ZXIyaHVudGVyMg=="),
	"bearer token header":        j("Authorization: ", "Bearer ", "aB3dE7zQ9wM2nK4pL6rT8vX1yU5iO0sD"),
}

func TestNewCategoriesDetected(t *testing.T) {
	for category, value := range newSamples {
		f, err := ScanBytes("f.txt", []byte("context line\n"+value+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, x := range f {
			if x.Category == category {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not detected in %q, got %+v", category, value, f)
		}
	}
}

func TestNewValuesAreRedacted(t *testing.T) {
	for category, value := range newSamples {
		out := Redact("before " + value + " after")
		if strings.Contains(out, value) {
			t.Errorf("%s: the value survived redaction", category)
		}
	}
}

// A guard that fires on ordinary files gets switched off. These are the
// kinds of content a repository is full of, and none of it is a secret.
func TestOrdinaryRepositoryContentIsNotReported(t *testing.T) {
	corpus := map[string]string{
		"a go module checksum": "github.com/stretchr/testify v1.8.4 h1:CcVxjf4T2CAHzgZ3pWnNnwcvaKw1QEzKrO3zJtQZiD8=\n" +
			"github.com/stretchr/testify v1.8.4/go.mod h1:sz/lmYIOXD5VzOJnkRt0/lVxBddaHzTbsdvM4JLZAI0=",
		"a package lock entry": `"resolved": "https://registry.npmjs.org/react/-/react-18.2.0.tgz",
"integrity": "sha512-/3IjMdb2L9QbBdWiW5e3P2/npwMBaU9mHCSCUzNln0ZCYbcfTsGbTJrU/kGemdH2IWmB2ioZ+zkxtmq6g09fGQ=="`,
		"commit hashes":  "parent 8e9a0cdb46b0f8f5ab54cbb1f9a3b2c10d7e4a21\ntree 4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		"identifiers":    `{"id": "550e8400-e29b-41d4-a716-446655440000", "trace": "0af7651916cd43dd8448eb211c80319c"}`,
		"a data uri":     `<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk">`,
		"example values": "password = \"changeme\"\napi_key: \"your-api-key-here\"\ntoken = \"<token>\"\nsecret = \"${VAULT_SECRET}\"",
		"documentation": "Set the AWS_SECRET_ACCESS_KEY environment variable before running.\n" +
			"The authorization header uses a bearer token that the server issues.",
		"code about credentials": `func validatePassword(password string) error {
	if len(password) < 12 { return errors.New("too short") }
	return nil
}
const tokenHeader = "Authorization"`,
		"a sentry style url":   `SENTRY_DSN=https://examplePublicKey@o0.ingest.sentry.io/0`,
		"a normal remote url":  "origin\tgit@github.com:example/app.git (fetch)",
		"minified javascript":  `function a(b){return b.replace(/[A-Za-z0-9+/=]{20,}/g,"")}var c="aaaaaaaaaaaaaaaaaaaaaaaa";`,
		"a long hex constant":  "const checksum = 0x9e3779b97f4a7c15\nsha256: 2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
		"a base64 test vector": "want := \"aGVsbG8gd29ybGQgdGhpcyBpcyBhIHRlc3QgdmVjdG9y\"",
	}
	for name, content := range corpus {
		f, err := ScanBytes("sample", []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		if len(f) != 0 {
			t.Errorf("%s was reported as holding %v", name, Categories(f))
		}
	}
}

func BenchmarkScanFile(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString("func handler(w http.ResponseWriter, r *http.Request) { render(w, r.Context(), 200) }\n")
	}
	content := []byte(sb.String())
	b.SetBytes(int64(len(content)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ScanBytes("big.go", content); err != nil {
			b.Fatal(err)
		}
	}
}

// The prefilter skips a line that holds none of the literal anchors. A
// file full of words that do contain them, without ever being a
// credential, is the case where that shortcut does not help, so it is
// measured rather than assumed.
func BenchmarkScanFileWithNearMisses(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString(`password := os.Getenv("APP_PASSWORD") // see https://example.com/docs, key-value store` + "\n")
	}
	content := []byte(sb.String())
	b.SetBytes(int64(len(content)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ScanBytes("near.go", content); err != nil {
			b.Fatal(err)
		}
	}
}
