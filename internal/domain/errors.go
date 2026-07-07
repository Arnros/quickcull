package domain

// ErrorPrefix is prepended to all CodedError messages to make them
// easily identifiable when surfaced to the UI or logs.
const ErrorPrefix = "QCERR:"

// CodedError is a sentinel error that carries a stable string code.
// Codes are snake_case and never contain spaces, making them safe to
// compare across serialization boundaries (e.g. JSON API responses).
type CodedError struct {
	Code string
}

func (e CodedError) Error() string {
	return ErrorPrefix + e.Code
}

// NewCodedError creates a new CodedError with the given code.
// Prefer using the pre-declared Err* variables instead of creating new ones.
func NewCodedError(code string) error {
	return CodedError{Code: code}
}

var (
	ErrFolderNotFound        = CodedError{Code: "folder_not_found"}
	ErrNoMediaFiles          = CodedError{Code: "no_media_files"}
	ErrIndexOutOfBounds      = CodedError{Code: "index_out_of_bounds"}
	ErrNothingToUndo         = CodedError{Code: "nothing_to_undo"}
	ErrAccessDenied          = CodedError{Code: "access_denied"}
	ErrPathRequired          = CodedError{Code: "path_required"}
	ErrUnsupportedPlatform   = CodedError{Code: "unsupported_platform"}
	ErrInvalidRotationDir    = CodedError{Code: "invalid_rotation_direction"}
	ErrInvalidLabel          = CodedError{Code: "invalid_label"}
	ErrTrashFailed           = CodedError{Code: "trash_failed"}
	ErrExiftoolTimeout       = CodedError{Code: "exiftool_timeout"}
	ErrExiftoolApplyFailed   = CodedError{Code: "exiftool_apply_failed"}
	ErrExiftoolPathMustBeAbs = CodedError{Code: "exiftool_path_must_be_absolute"}
	ErrExifWriteUnsupported  = CodedError{Code: "exif_write_unsupported"}
	ErrUnknownActionType     = CodedError{Code: "unknown_action_type"}
	ErrInvalidCriteria       = CodedError{Code: "invalid_criteria"}
	ErrExportFailed          = CodedError{Code: "export_failed"}
	ErrConfigDirCreate       = CodedError{Code: "config_dir_create_failed"}
	ErrTrashCopyFailed       = CodedError{Code: "trash_copy_failed"}
	ErrTrashDirCreate        = CodedError{Code: "trash_dir_create_failed"}
	ErrInvalidPath           = CodedError{Code: "invalid_path"}
	ErrPersistenceInit       = CodedError{Code: "persistence_init_failed"}
	ErrLoadInProgress        = CodedError{Code: "load_in_progress"}
)
