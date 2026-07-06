package subcategory

import "testing"

func TestValidIcon_AcceptsEmoji(t *testing.T) {
	if !ValidIcon("🐶") {
		t.Error("ValidIcon(🐶) = false, want true")
	}
}

func TestValidIcon_RejectsEmpty(t *testing.T) {
	if ValidIcon("") {
		t.Error("ValidIcon(\"\") = true, want false")
	}
}

func TestValidIcon_RejectsPlainWord(t *testing.T) {
	if ValidIcon("perro") {
		t.Error("ValidIcon(perro) = true, want false")
	}
}

func TestValidIcon_RejectsSentence(t *testing.T) {
	if ValidIcon("mi categoria nueva") {
		t.Error("ValidIcon(sentence) = true, want false")
	}
}

func TestValidIcon_RejectsTooLong(t *testing.T) {
	if ValidIcon("🐶🐱🐭🐹🐰") {
		t.Error("ValidIcon(5 emoji) = true, want false (too long)")
	}
}
