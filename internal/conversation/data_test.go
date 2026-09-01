package conversation

import "testing"

func TestCopyData_NeverReturnsNil_SoCallersCanWriteToTheCopy(t *testing.T) {
	copied := CopyData(nil)
	if copied == nil {
		t.Fatal("CopyData(nil) devolvió nil; maps.Clone(nil) es nil y todo call site escribe sobre la copia, así que el panic aparece recién en la vuelta siguiente")
	}
	copied["k"] = "v"
}

func TestCopyData_IsACopy_NotTheSameMap(t *testing.T) {
	original := Data{"k": "v"}
	copied := CopyData(original)
	copied["k"] = "otro"
	if original["k"] != "v" {
		t.Fatalf("CopyData devolvió el mismo map: escribir en la copia cambió el original a %v", original["k"])
	}
}
