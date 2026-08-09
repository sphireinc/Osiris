package osiris

import "testing"

// Result and Option remain distinct native ABI containers. This
// fixture exercises construction, printing, member inspection, and postfix
// propagation through generated C rather than only checking emitted source.
func TestNativeOptionOptionABIParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeOption-native-option-abi",
		Source: `emit some(41)
emit none()
emit issome(some(41))
emit isnone(none())
emit unwrap(some(41))
emit or_else(none(), 9)
emit issome(to_option(ok(7)))
`,
	})
}
