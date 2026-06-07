import { i18n } from "./i18n.svelte";
import { translations } from "./translations";

const BACKEND_ERROR_PREFIX = "QCERR:";

const errorCodeToTranslationKey: Record<string, keyof typeof translations["en"]> = {
  folder_not_found: "folder_not_found",
  no_media_files: "no_media_found",
  index_out_of_bounds: "index_out_of_bounds",
  nothing_to_undo: "nothing_to_undo",
  access_denied: "access_denied",
  path_required: "path_required",
  unsupported_platform: "unsupported_platform",
  invalid_rotation_direction: "invalid_rotation_direction",
  invalid_label: "invalid_label",
  trash_failed: "trash_failed",
  exiftool_timeout: "exiftool_timeout",
  exiftool_apply_failed: "exiftool_apply_failed",
  exiftool_path_must_be_absolute: "exiftool_path_must_be_absolute",
  exif_write_unsupported: "exif_write_unsupported",
  unknown_action_type: "generic_backend_error"
};

export function backendErrorCode(message: string): string | null {
  if (!message.startsWith(BACKEND_ERROR_PREFIX)) {
    return null;
  }
  return message.slice(BACKEND_ERROR_PREFIX.length);
}

export function localizeBackendError(message: string): string {
  const code = backendErrorCode(message);
  if (!code) {
    return message;
  }

  const key = errorCodeToTranslationKey[code];
  if (!key) {
    return i18n.t("generic_backend_error");
  }
  return i18n.t(key as any);
}
