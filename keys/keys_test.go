package keys

import (
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

// A user working in Russian would otherwise have to switch layout for every shortcut. The
// keys are matched by physical position: on a ЙЦУКЕН keyboard the key labelled В is the one
// labelled D on QWERTY, so pressing it means the same thing.
func TestCyrillicKeysResolveToTheSameAction(t *testing.T) {
	for _, tc := range []struct{ latin, cyrillic string }{
		{"n", "т"}, {"N", "Т"},
		{"D", "В"},
		{"o", "щ"},
		{"p", "з"},
		{"c", "с"},
		{"r", "к"}, {"R", "К"},
		{"q", "й"},
		{"j", "о"}, {"k", "л"},
		{"J", "О"}, {"K", "Л"},
	} {
		want, ok := GetKeyName(tc.latin)
		require.True(t, ok, "precondition: %q is a known binding", tc.latin)

		got, ok := GetKeyName(tc.cyrillic)
		require.True(t, ok, "%q sits on the same physical key as %q and should be recognised",
			tc.cyrillic, tc.latin)
		require.Equal(t, want, got, "%q should do what %q does", tc.cyrillic, tc.latin)
	}
}

// Layout support is additive: every existing binding must behave exactly as it did.
func TestLatinKeysAreUnchanged(t *testing.T) {
	for key, want := range GlobalKeyStringsMap {
		got, ok := GetKeyName(key)
		require.True(t, ok, "%q should still resolve", key)
		require.Equal(t, want, got, "%q should keep its action", key)
	}
}

// Keys bound in neither alphabet must stay unbound, or the UI would act on arbitrary typing.
func TestUnknownKeysStayUnbound(t *testing.T) {
	for _, key := range []string{"z", "я", "ж", "ctrl+c", "f1", ""} {
		_, ok := GetKeyName(key)
		require.False(t, ok, "%q should not be bound to anything", key)
	}
}

// The guard that matters over time: a binding added later on a physical key the layout map
// does not cover would ship working for Latin users only, and nothing would say so.
func TestEveryLetterBindingIsReachableFromCyrillic(t *testing.T) {
	latinToCyrillic := make(map[string]string, len(cyrillicToLatin))
	for cyrillic, latin := range cyrillicToLatin {
		latinToCyrillic[latin] = cyrillic
	}

	for key := range GlobalKeyStringsMap {
		runes := []rune(key)
		if len(runes) != 1 || !unicode.IsLetter(runes[0]) {
			// Named keys ("up", "tab") and punctuation ("?") are layout-independent.
			continue
		}
		require.Contains(t, latinToCyrillic, key,
			"binding %q has no Cyrillic equivalent: add its physical key to cyrillicToLatin", key)
	}
}
