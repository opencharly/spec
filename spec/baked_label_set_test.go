package spec

import (
	"reflect"
	"testing"
)

// baked_label_set_test.go — the build-side label carrier has no skill-specific field
// (opencharly/sdk#598): kind:skill's ai.opencharly.skill label arrives through the generic
// Contributed carrier, and the deploy-side reader (BoxMetadata.Skills) is unchanged. The removal
// is asserted on the GENERATED struct, because the CUE once declared `skills` twice in
// #BakedLabelSet and deleting one declaration left the field alive (CUE unifies the repeat).
func TestBakedLabelSet_SkillsRideTheGenericContributedCarrier(t *testing.T) {
	baked := reflect.TypeOf(BakedLabelSet{})
	if _, ok := baked.FieldByName("Skills"); ok {
		t.Fatal("BakedLabelSet still declares Skills: the skill label must be contributed by kind:skill, not a typed field")
	}
	if f, ok := baked.FieldByName("Contributed"); !ok || f.Type != reflect.TypeOf(map[string]RawBody{}) {
		t.Fatalf("BakedLabelSet.Contributed = %v (present %v), want map[string]RawBody", f.Type, ok)
	}
	if _, ok := reflect.TypeOf(BoxMetadata{}).FieldByName("Skills"); !ok {
		t.Fatal("BoxMetadata.Skills is gone: the deploy-side reader of ai.opencharly.skill must stay")
	}
}
