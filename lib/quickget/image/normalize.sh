# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Image: post-download normalisation — decompress/convert the artifact
# into its final on-disk form per OS; updates the ISO global.

function image_normalize() {
    case "${OS}" in
        batocera)
            if [[ ${ISO} = *".gz"* ]]; then
                gzip -d "${VM_PATH}/${ISO}"
                ISO="${ISO/.gz/}"
            fi
            # Resize the raw image to provide space for Batocera's partition expansion
            require_qemu_img
            ${QEMU_IMG} resize -f raw "${VM_PATH}/${ISO}" 128G;;
        dragonflybsd)
            #  Could be other OS iso files compressed with bzip2 or gzip
            #  but for now we'll keep this to know cases
            if [[ ${ISO} = *".bz2"* ]]; then
                bzip2 -d  "${VM_PATH}/${ISO}"
                ISO="${ISO/.bz2/}"
            fi;;
        easyos)
            if [[ ${ISO} = *".img"* ]]; then
                require_qemu_img
                ${QEMU_IMG} convert -f raw -O qcow2 "${VM_PATH}/${ISO}" "${VM_PATH}/disk.qcow2"
                ISO="${ISO/.img/}"
            fi;;
        openfyde)
            if [[ ${ISO} = *".xz"* ]]; then
                xz -d "${VM_PATH}/${ISO}"
                ISO="${ISO/.xz/}"
                require_qemu_img
                ${QEMU_IMG} convert -f raw -O qcow2 "${VM_PATH}/${ISO}" "${VM_PATH}/disk.qcow2"
                ISO="disk.qcow2"
            fi;;
        redoxos)
            if [[ ${ISO} = *".zst"* ]]; then
                if [ -z "$(command -v zstd)" ]; then echo "ERROR! 'zstd' needs installing. Unable to extract file."
                else
                    zstd -dq "${VM_PATH}/${ISO}"
                    rm -f "${VM_PATH}/${ISO}"
                    ISO="${ISO/.zst/}"
                fi
            fi;;
        freedos)
            if [[ ${ISO} = *".zip"* ]]; then
                unzip -qo "${VM_PATH}/${ISO}" -d "${VM_PATH}"
                rm -f "${VM_PATH}/${ISO}"
                ISO="$(ls -1 "${VM_PATH}/"*.iso)"
                ISO="$(basename "${ISO}")"
            fi;;
        kolibrios)
            if [[ ${ISO} = *".7z" ]]; then
                if [ -z "$(command -v 7z)" ]; then echo "ERROR! '7zip' needs installing. Unable to extract file."
                else
                    7z e "${VM_PATH}/${ISO}" -o"${VM_PATH}" >/dev/null 2>&1
                    rm -f "${VM_PATH}/${ISO}"
                    ISO="$(ls -1 "${VM_PATH}/"*.iso)"
                    ISO="$(basename "${ISO}")"
                fi
            fi;;
        reactos)
            if [[ ${ISO} = *".zip"* ]]; then
                unzip -qo "${VM_PATH}/${ISO}" -d "${VM_PATH}"
                rm -f "${VM_PATH}/${ISO}"
                ISO="$(ls -1 "${VM_PATH}/"*.iso)"
                ISO="$(basename "${ISO}")"
            fi;;
    esac
}
