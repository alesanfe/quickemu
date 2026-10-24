# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Image: generated seed/autoinstall ISO builders.


# Generate a NoCloud/cloud-init seed.iso inside the VM directory.
# Usage: gen_seed_iso <vm_path> [user-data-file]
function gen_seed_iso() {
    local VM_PATH="${1}"
    local USER_DATA="${2:-}"
    local USER_NAME="${USER:-quickemu}"
    local SSH_KEY=""
    SSH_KEY=$(cat "${HOME}/.ssh/id_ed25519.pub" "${HOME}/.ssh/id_rsa.pub" 2>/dev/null | head -1)
    if [ -z "${USER_DATA}" ]; then
        USER_DATA="${VM_PATH}/user-data"
        cat << EOF > "${USER_DATA}"
#cloud-config
hostname: ${VM_PATH}
manage_etc_hosts: true
users:
  - name: ${USER_NAME}
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: sudo
    shell: /bin/bash
    lock_passwd: false
    plain_text_passwd: quickemu
${SSH_KEY:+    ssh_authorized_keys:
      - ${SSH_KEY}}
ssh_pwauth: true
package_update: true
EOF
    fi
    if [ ! -f "${VM_PATH}/meta-data" ]; then
        cat << EOF > "${VM_PATH}/meta-data"
instance-id: ${VM_PATH}
local-hostname: ${VM_PATH}
EOF
    fi
    if command -v cloud-localds >/dev/null 2>&1; then
        cloud-localds "${VM_PATH}/seed.iso" "${USER_DATA}" "${VM_PATH}/meta-data"
    elif command -v genisoimage >/dev/null 2>&1; then
        genisoimage -output "${VM_PATH}/seed.iso" -volid cidata -joliet -rock "${USER_DATA}" "${VM_PATH}/meta-data" 2>/dev/null
    elif command -v xorrisofs >/dev/null 2>&1; then
        xorrisofs -output "${VM_PATH}/seed.iso" -volid cidata -joliet -rock "${USER_DATA}" "${VM_PATH}/meta-data" 2>/dev/null
    elif command -v mkisofs >/dev/null 2>&1; then
        mkisofs -output "${VM_PATH}/seed.iso" -volid cidata -joliet -rock "${USER_DATA}" "${VM_PATH}/meta-data" 2>/dev/null
    else
        echo "ERROR! No tool to build seed.iso (need cloud-localds, genisoimage, xorrisofs or mkisofs)."
        return 1
    fi
    if [ -f "${VM_PATH}/seed.iso" ]; then
        echo "seed=\"${VM_PATH}/seed.iso\"" >> "${CONF_FILE}"
        echo " - NoCloud seed.iso created (user: ${USER_NAME}, password: quickemu)"
    fi
}


# Extract kernel+initrd from an ISO for direct kernel boot (unattended installs).
# Usage: iso_extract_bootfiles <iso> <out_dir> <member_prefix>
function iso_extract_bootfiles() {
    local ISO="${1}" OUT="${2}" PREFIX="${3}"
    if command -v bsdtar >/dev/null 2>&1; then
        bsdtar -C "${OUT}" -xf "${ISO}" "${PREFIX}/vmlinuz" "${PREFIX}/initrd" 2>/dev/null
    elif command -v 7z >/dev/null 2>&1; then
        7z e -o"${OUT}" "${ISO}" "${PREFIX}/vmlinuz" "${PREFIX}/initrd" >/dev/null 2>&1
    elif command -v isoinfo >/dev/null 2>&1; then
        isoinfo -i "${ISO}" -x "/${PREFIX^^}/VMLINUZ.;1" > "${OUT}/vmlinuz" 2>/dev/null
        isoinfo -i "${ISO}" -x "/${PREFIX^^}/INITRD.;1" > "${OUT}/initrd" 2>/dev/null
    else
        echo "ERROR! Need bsdtar, 7z or isoinfo to extract kernel from the ISO."
        return 1
    fi
    # Normalise extracted names (7z/bsdtar may place them at OUT root or under PREFIX).
    if [ ! -s "${OUT}/vmlinuz" ] && [ -s "${OUT}/${PREFIX}/vmlinuz" ]; then
        mv "${OUT}/${PREFIX}/vmlinuz" "${OUT}/vmlinuz"
        mv "${OUT}/${PREFIX}/initrd" "${OUT}/initrd" 2>/dev/null
    fi
    [ -s "${OUT}/vmlinuz" ] && [ -s "${OUT}/initrd" ]
}


# Ubuntu autoinstall (subiquity) via direct kernel boot + NoCloud seed.
function gen_ubuntu_autoinstall() {
    local VM_PATH="${1}" ISO="${2}"
    if ! iso_extract_bootfiles "${ISO}" "${VM_PATH}" "casper"; then
        echo "ERROR! Could not extract casper/vmlinuz+initrd from ${ISO}."
        return 1
    fi
    local USER_NAME="${USER:-quickemu}" PWHASH=""
    if command -v openssl >/dev/null 2>&1; then
        PWHASH=$(openssl passwd -6 "quickemu")
    else
        echo "ERROR! openssl is required to generate the install password hash."
        return 1
    fi
    cat << EOF > "${VM_PATH}/user-data"
#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: ${VM_PATH}
    username: ${USER_NAME}
    password: "${PWHASH}"
  ssh:
    install-server: true
    allow-pw: true
  storage:
    layout:
      name: direct
  late-commands:
    - echo "${USER_NAME} ALL=(ALL) NOPASSWD:ALL" > /target/etc/sudoers.d/${USER_NAME}
EOF
    gen_seed_iso "${VM_PATH}" "${VM_PATH}/user-data"
    {
        echo "kernel=\"${VM_PATH}/vmlinuz\""
        echo "initrd=\"${VM_PATH}/initrd\""
        # ds=nocloud reads the attached seed; autoinstall triggers subiquity unattended mode.
        echo "boot_args=\"autoinstall ds=nocloud;s=/cdrom/\""
    } >> "${CONF_FILE}"
    echo " - Unattended install configured (user: ${USER_NAME}, password: quickemu)"
}
