package transforms

import (
	"testing"
)

func TestStringsTransforms(t *testing.T) {
	if got := RemoveAccents("Olá, você está aí? Cachaça"); got != "Ola, voce esta ai? Cachaca" {
		t.Fatalf("RemoveAccents failed: %q", got)
	}

	if got := RemoveSpecialChars("hello@world#2026!"); got != "helloworld2026" {
		t.Fatalf("RemoveSpecialChars failed: %q", got)
	}

	if got := ToSnakeCase("userAccountIdentifier"); got != "user_account_identifier" {
		t.Fatalf("ToSnakeCase failed: %q", got)
	}

	if got := ToCamelCase("user_account_identifier"); got != "userAccountIdentifier" {
		t.Fatalf("ToCamelCase failed: %q", got)
	}

	if got := ToKebabCase("UserAccountIdentifier"); got != "user-account-identifier" {
		t.Fatalf("ToKebabCase failed: %q", got)
	}

	if got := TransformKey("Configuração Básica!"); got != "configuracao_basica" {
		t.Fatalf("TransformKey failed: %q", got)
	}
}

func TestDurations(t *testing.T) {
	durStr := "1d 2h 30m 15s"
	secs := StringToSeconds(durStr)
	expected := int64(86400 + 7200 + 1800 + 15)
	if secs != expected {
		t.Fatalf("expected %d, got %d", expected, secs)
	}

	formatted := SecondsToFormattedString(secs)
	if formatted != "1d 2h 30m 15s" {
		t.Fatalf("expected 1d 2h 30m 15s, got %s", formatted)
	}
}

func TestSliceOperations(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7}
	chunks := Chunk(items, 3)
	if len(chunks) != 3 || len(chunks[0]) != 3 || len(chunks[2]) != 1 {
		t.Fatalf("Chunk failed: %v", chunks)
	}

	duplicates := []string{"a", "b", "a", "c", "b"}
	uniq := Unique(duplicates)
	if len(uniq) != 3 || uniq[0] != "a" || uniq[1] != "b" || uniq[2] != "c" {
		t.Fatalf("Unique failed: %v", uniq)
	}

	diff := Difference([]int{1, 2, 3, 4}, []int{3, 4, 5})
	if len(diff) != 2 || diff[0] != 1 || diff[1] != 2 {
		t.Fatalf("Difference failed: %v", diff)
	}

	intersect := Intersection([]int{1, 2, 3, 4}, []int{3, 4, 5})
	if len(intersect) != 2 || intersect[0] != 3 || intersect[1] != 4 {
		t.Fatalf("Intersection failed: %v", intersect)
	}
}

func TestDeepMerge(t *testing.T) {
	dst := map[string]any{
		"a": 1,
		"nested": map[string]any{
			"x": "initial",
			"y": 10,
		},
	}
	src := map[string]any{
		"b": 2,
		"nested": map[string]any{
			"x": "updated",
			"z": true,
		},
	}
	merged := DeepMerge(dst, src)
	nested := merged["nested"].(map[string]any)
	if nested["x"] != "updated" || nested["y"] != 10 || nested["z"] != true || merged["a"] != 1 || merged["b"] != 2 {
		t.Fatalf("DeepMerge failed: %v", merged)
	}
}
