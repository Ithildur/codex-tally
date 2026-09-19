package dashboard

import (
	"cmp"
	_ "embed"
	json "encoding/json/v2"
	"os"
	"strings"
)

//go:embed public/i18n.json
var englishJSON []byte

var english = func() map[string]string {
	var messages map[string]string
	if err := json.Unmarshal(englishJSON, &messages); err != nil {
		panic(err)
	}
	return messages
}()

func (v publicView) Text(message string) string {
	if v.Language == "en" {
		if translated, ok := english[message]; ok {
			return translated
		}
	}
	return message
}

func consoleText(message string) string {
	locale := cmp.Or(os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"))
	if strings.HasPrefix(strings.ToLower(locale), "zh") {
		return message
	}
	if translated, ok := english[message]; ok {
		return translated
	}
	return message
}

// Translate at the command boundary while preserving errors.Is/errors.As.
type commandError struct {
	cause   error
	message string
}

func (e commandError) Error() string { return e.message }
func (e commandError) Unwrap() error { return e.cause }
