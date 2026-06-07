import { i18n } from "./i18n.svelte";

const rawNoExiftoolCameraValues = new Set([
  "__raw_no_exiftool__",
  "RAW (No Exiftool)",
]);
const rawMetadataUnavailableCameraValues = new Set([
  "__raw_metadata_unavailable__",
]);

export function cameraLabel(camera: string): string {
  if (rawNoExiftoolCameraValues.has(camera)) {
    return i18n.t("raw_no_exiftool_camera");
  }
  if (rawMetadataUnavailableCameraValues.has(camera)) {
    return i18n.t("raw_metadata_unavailable_camera");
  }
  return camera;
}
