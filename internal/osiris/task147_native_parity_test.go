package osiris

import "testing"

func TestTask147NativeStandardLibraryParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task147-native-standard-library",
		Source: `import "std/text" as text
import "std/list" as list
import "std/signal" as signal
import "std/crypto" as crypto
import "std/time" as time
emit text.trim("  Café  ")
emit text.split("a,b,c", ",")
emit list.reverse([1, 2, 3])
emit signal.mean([1, 2, 3])
emit crypto.digest("hello", "sha256")
emit time.duration_string(time.parse_duration("2s"))
`,
	})
}
