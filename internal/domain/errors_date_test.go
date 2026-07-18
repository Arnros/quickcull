package domain

import "testing"

func TestCodedErrorHelpers(t *testing.T) {
	if ErrFolderNotFound.Error() != "QCERR:folder_not_found" {
		t.Fatalf("unexpected predefined coded error: %q", ErrFolderNotFound.Error())
	}
}
