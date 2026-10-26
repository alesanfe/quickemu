# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: per-OS download URL builders.


function get_alma() {
    local HASH=""
    local QEMU_ARCH="x86_64"
    [ "${ARCH}" == "arm64" ] && QEMU_ARCH="aarch64"
    local ISO="AlmaLinux-${RELEASE}-latest-${QEMU_ARCH}-${EDITION}.iso"
    local URL="https://repo.almalinux.org/almalinux/${RELEASE}/isos/${QEMU_ARCH}"
    HASH="$(web_pipe "${URL}/CHECKSUM" | grep "(${ISO}" | cut -d' ' -f4)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_alpine() {
    local HASH=""
    local ISO=""
    local QEMU_ARCH="x86_64"
    [ "${ARCH}" == "arm64" ] && QEMU_ARCH="aarch64"
    local URL="https://dl-cdn.alpinelinux.org/alpine/${RELEASE}/releases/${QEMU_ARCH}"
    local VERSION=""
    VERSION=$(web_pipe "${URL}/latest-releases.yaml" | awk '/"Xen"/{found=0} {if(found) print} /"Virtual"/{found=1}' | grep 'version:' | head -1 | awk '{print $2}')
    ISO="alpine-virt-${VERSION}-${QEMU_ARCH}.iso"
    HASH=$(web_pipe "${URL}/latest-releases.yaml" | awk '/"Xen"/{found=0} {if(found) print} /"Virtual"/{found=1}' | grep 'sha256:' | head -1 | awk '{print $2}')
    echo "${URL}/${ISO} ${HASH}"
}


function get_android() {
    local HASH=""
    local ISO=""
    local JSON_ALL=""
    local JSON_REL=""
    local URL="https://mirrors.gigenet.com/OSDN/android-x86"
    JSON_ALL=$(web_pipe "https://www.fosshub.com/Android-x86-old.html" | grep "var settings =" | cut -d'=' -f2-)
    JSON_REL=$(echo "${JSON_ALL}" | jq --arg ver "${OS}-${EDITION}-${RELEASE}" 'first(.pool.f[] | select((.n | startswith($ver)) and (.n | endswith(".iso"))))')
    ISO=$(echo "${JSON_REL}" | jq -r .n)
    HASH=$(echo "${JSON_REL}" | jq -r .hash.sha256)
    # Traverse the directories to find the .iso location
    for DIR in $(web_pipe "${URL}" | grep -o -E '[0-9]{5}' | sort -ur); do
        if web_pipe "${URL}/${DIR}" | grep "${ISO}" &>/dev/null; then
            URL="${URL}/${DIR}"
            break
        fi
    done
    echo "${URL}/${ISO} ${HASH}"
}


function get_antix() {
    local HASH=""
    local ISO="antiX-${RELEASE}"
    local README="README"
    local URL="https://sourceforge.net/projects/antix-linux/files/Final/antiX-${RELEASE}"

    # antiX uses a different URL and ISO naming for runit editions
    if [[ "${EDITION}" == *"runit"* ]];then
        ISO+="-runit"
        README="README2"
        case ${RELEASE} in
            21) URL+="/runit-bullseye";;
            *)  URL+="/runit-antiX-${RELEASE}";;
        esac
    fi
    case ${EDITION} in
        base-*) ISO+="_x64-base.iso";;
        core-*) ISO+="_x64-core.iso";;
        full-*) ISO+="_x64-full.iso";;
        net-*)  ISO+="-net_x64-net.iso";;
    esac
    HASH=$(web_pipe "${URL}/${README}.txt" | grep "${ISO}" | cut -d' ' -f1 | head -n 1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_archcraft() {
    local HASH=""
    local ISO=""
    local URL=""
    local VERSION_FOLDER=""
    URL="https://sourceforge.net/projects/archcraft/files/${RELEASE}/download"
    URL="$(web_redirect "${URL}" | cut -d? -f1)"
    ISO="$(basename "${URL}")"
    VERSION_FOLDER="$(dirname "${URL}" | xargs basename)"
    HASH=$(web_pipe "https://sourceforge.net/projects/archcraft/files/${VERSION_FOLDER}/${ISO}.sha256sum" | cut -d' ' -f1)
    echo "${URL} ${HASH}"
}


function get_archlinux() {
    local HASH=""
    local ISO=""
    local URL="https://geo.mirror.pkgbuild.com/"
    ISO=$(web_pipe "https://archlinux.org/releng/releases/json/" | jq -r '.releases[0].iso_url')
    HASH=$(web_pipe "https://archlinux.org/releng/releases/json/" | jq -r '.releases[0].sha256_sum')
    echo "${URL}${ISO} ${HASH}"
}


function get_artixlinux() {
    local HASH=""
    local ISO=""
    local URL="https://iso.artixlinux.org/iso"
    ISO="artix-${EDITION}-${RELEASE}-x86_64.iso"
    HASH=$(web_pipe "${URL}/sha256sums" | grep "${ISO}")
    echo "${URL}/${ISO} ${HASH}"
}


function get_azurelinux() {
    local QEMU_ARCH="x86_64"
    [ "${ARCH}" == "arm64" ] && QEMU_ARCH="aarch64"
    local URL="https://aka.ms/azurelinux-${RELEASE}-${QEMU_ARCH}.iso"
    local ISO
    ISO="$(web_redirect "${URL}")"
    echo "${ISO}"
}


function get_batocera() {
    local HASH=""
    local ISO=""
    local URL="https://mirrors.o2switch.fr/batocera/x86_64/stable/${RELEASE}"
    ISO="$(web_pipe "${URL}/" | grep -e 'batocera.*img.gz'| cut -d'"' -f2)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_bazzite() {
    local HASH=""
    local ISO=""
    local URL="https://download.bazzite.gg"
    case ${EDITION} in
        gnome) ISO="bazzite-gnome-stable-amd64.iso";;
        plasma)  ISO="bazzite-stable-amd64.iso";;
        deck-gnome)  ISO="bazzite-deck-gnome-stable-amd64.iso";;
        deck-plasma)  ISO="bazzite-deck-stable-amd64.iso";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}-CHECKSUM" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_biglinux() {
    local HASH=""
    local ISO="biglinux_${RELEASE}_${EDITION}.iso"
    local URL="https://iso.biglinux.com.br"
    HASH=$(web_pipe "${URL}/${ISO}.md5" | grep -Eo '[[:alnum:]]{32}')
    echo "${URL}/${ISO} ${HASH}"
}


function get_blendos() {
    local HASH=""
    local ISO="blendOS.iso"
    # Use the official GitLab build server
    local URL="https://git.blendos.co/api/v4/projects/32/jobs/artifacts/main/raw/blendOS.iso?job=build-job"
    HASH=$(web_pipe "https://git.blendos.co/api/v4/projects/32/jobs/artifacts/main/raw/checksum?job=build-job" | cut -d' ' -f1)
    echo "${URL} ${HASH}"
}


function get_bodhi() {
    local HASH=""
    local ISO=""
    local URL="https://sourceforge.net/projects/bodhilinux/files/${RELEASE}"
    case ${EDITION} in
        standard) ISO="bodhi-${RELEASE}-64.iso";;
        *) ISO="bodhi-${RELEASE}-64-${EDITION}.iso";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_bunsenlabs() {
    local HASH=""
    local ISO="boron-1-240123-amd64.hybrid.iso"
    local URL="https://ddl.bunsenlabs.org/ddl"
    HASH=$(web_pipe "${URL}/release.sha256.txt" | head -n 1 | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_cachyos() {
    local HASH=""
    local URL=""
    URL="$(web_pipe "https://cachyos.org/download/" | tr '&' '\n' | grep "ISO/${EDITION}" | grep -v 'iso.sha' | grep -v 'iso.sig' | cut -d';' -f2)"
    HASH=$(web_pipe "${URL}.sha256" | cut -d' ' -f1)
    echo "${URL} ${HASH}"
}


function get_centos-stream() {
    local HASH=""
    local ISO="CentOS-Stream-${RELEASE}-latest-x86_64-${EDITION}.iso"
    local URL="https://linuxsoft.cern.ch/centos-stream/${RELEASE}-stream/BaseOS/x86_64/iso"
    HASH=$(web_pipe "${URL}/${ISO}.SHA256SUM" | grep "SHA256 (${ISO}" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_chimeralinux() {
    local DATE=""
    local HASH=""
    local URL="https://repo.chimera-linux.org/live/${RELEASE}"
    DATE=$(web_pipe "${URL}/sha256sums.txt" | head -n1 | cut -d'-' -f5)
    local ISO="chimera-linux-x86_64-LIVE-${DATE}-${EDITION}.iso"
    HASH=$(web_pipe "${URL}/sha256sums.txt" | grep 'x86_64-LIVE' | grep "${EDITION}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_crunchbang++() {
    local HASH=""
    local ISO=""
    ISO=$(web_pipe "https://api.github.com/repos/CBPP/cbpp/releases" | grep 'download_url' | grep amd64 | grep "${RELEASE}" | cut -d'"' -f4)
    echo "${ISO} ${HASH}"
}


function get_debian() {
    local DEBIAN_ARCH="${ARCH}"
    local DEBCURRENT=""
    local HASH=""
    local ISO="debian-live-${RELEASE}-${DEBIAN_ARCH}-${EDITION}.iso"
    local URL="https://cdimage.debian.org/cdimage/archive/${RELEASE}-live/${DEBIAN_ARCH}/iso-hybrid"

    # Debian only provides netinst images for ARM64
    if [ "${DEBIAN_ARCH}" == "arm64" ] && [ "${EDITION}" != "netinst" ]; then
        echo "ERROR! Debian ${EDITION} is not available for ARM64. Use 'netinst' edition."
        exit 1
    fi

    DEBCURRENT=$(web_pipe "https://cdimage.debian.org/debian-cd/" | grep '\.[0-9]/' | cut -d'>' -f 9 | cut -d'/' -f 1)
    case "${RELEASE}" in
        "${DEBCURRENT}") URL="https://cdimage.debian.org/debian-cd/${RELEASE}-live/${DEBIAN_ARCH}/iso-hybrid";;
    esac
    if [ "${EDITION}" == "netinst" ]; then
        URL="${URL/-live/}"
        URL="${URL/hybrid/cd}"
        ISO="${ISO/-live/}"
    fi
    HASH=$(web_pipe "${URL}/SHA512SUMS" | grep "${ISO}" | cut -d' ' -f1 | head -n 1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_deepin() {
    local HASH=""
    local REV=${RELEASE}
    # deepin-desktop-community-20.3-amd64.iso
    local URL="https://cdimage.deepin.com/releases/"${RELEASE}
    # Correct URL for 23-RC onwards which has architecture directories
    if [ "${RELEASE}" != "20.9" ]; then
        URL+="/amd64"
    fi
    local ISO="deepin-desktop-community-${REV}-amd64.iso"
    HASH=$(web_pipe "${URL}/SHA256SUMS" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_devuan() {
    local HASH=""
    local ISO=""
    local URL="https://files.devuan.org/devuan_${RELEASE}/desktop-live"
    local VER=""
    case ${RELEASE} in
        beowulf)  VER="3.1.1";;
        chimaera) VER="4.0.3";;
        daedalus) VER="5.0.0";;
    esac
    ISO="devuan_${RELEASE}_${VER}_amd64_desktop-live.iso"
    HASH=$(web_pipe "${URL}/SHASUMS.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_dragonflybsd() {
    local HASH=""
    local ISO="dfly-x86_64-${RELEASE}_REL.iso.bz2"
    local URL="http://mirror-master.dragonflybsd.org/iso-images"
    HASH=$(web_pipe "${URL}/md5.txt" | grep "(${ISO})" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_dragonos() {
    local HASH=""
    local ISO="DragonOS_${RELEASE}.iso"
    local URL="https://downloads.sourceforge.net/project/dragonos-focal"
    echo "${URL}/${ISO} ${HASH}"
}


function get_easyos() {
    local HASH=""
    local URL=""
    local ISO=""
    local YEAR=""
    ISO="easy-${RELEASE}-amd64.img"
    TWO_YEARS=$(web_pipe https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/ | grep -o -E '[[:digit:]]{4}/' | sort -nr | tr -d /  | head -n 2 )
    for YEAR in ${TWO_YEARS} ; do
        if web_check "https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/${YEAR}/${RELEASE}/" ; then
            URL="https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/${YEAR}/${RELEASE}"
            HASH=$(web_pipe "${URL}/md5.sum.txt" | cut -d' ' -f1)
            break
        fi
    done
    echo "${URL}/${ISO} ${HASH}"
}


function get_elementary() {
    # Hash and ISO filename can be obtained from the below URL
    # https://elementary.io/docs/installation#verify-your-download
    local HASH=""
    local STAMP
    case ${RELEASE} in
        7.0) STAMP=".20230129rc";;
        7.1) STAMP=".20230926rc";;
        8.0) STAMP=".20241122rc";;
        8.1)
            STAMP="-amd64.20251211"
            HASH="eee6cad081664717681bec767fbfe1aa1fd920938fedad6c83b41fd341e8f306"
            ;;
    esac

    local ISO="elementaryos-${RELEASE}-stable${STAMP}.iso"
    local URL="https://ams3.dl.elementary.io/download"
    echo "${URL}/$(date +%s | base64)/${ISO} ${HASH}"
}


function get_endeavouros() {
    local ENDEAVOUR_RELEASES=""
    local HASH=""
    local ISO=""
    local URL="https://mirror.alpix.eu/endeavouros/iso"
    # Find EndeavourOS releases from mirror, pick one matching release
    ENDEAVOUR_RELEASES="$(web_pipe "${URL}/" | grep -o '<a href="[^"]*.iso">' | sed 's/^<a href="//;s/.iso">.*//' | grep -v 'x86_64')"
    ISO="$(echo "${ENDEAVOUR_RELEASES}" | grep -i "${RELEASE}").iso"
    HASH=$(web_pipe "${URL}/${ISO}.sha512sum" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_endless() {
    local HASH="" # No hash - there is a signature in .asc signed by
    #https://d1anzknqnc1kmb.cloudfront.net/eos-image-keyring.gpg
    # (4096R: CB50 0F7B C923 3FAD 32B4 E720 9E0C 1250 587A 279C)
    local FILE_TS=""
    # https://support.endlessos.org/en/installation/direct-download gives the info but computes the URLS in js
    # so parsing out the useful info is not happening tonight
    # Endless edition names are "base" for the small minimal one or the Language for the large full release
    # The isos are stamped as they are finished so ....
    case ${EDITION} in
        base)  FILE_TS="241023-183516";;
        en)    FILE_TS="241023-200926";;
        es)    FILE_TS="241023-184649";;
        fr)    FILE_TS="241023-191212";;
        pt_BR) FILE_TS="241023-191427";;
    esac
    URL="https://images-dl.endlessm.com/release/${RELEASE}/eos-amd64-amd64/${EDITION}"
    ISO="eos-eos${RELEASE:0:3}-amd64-amd64.${FILE_TS}.${EDITION}.iso"
    echo "${URL}/${ISO} ${HASH}"
}


function get_fedora() {
    local FEDORA_ARCH="x86_64"
    local HASH=""
    local ISO=""
    local JSON=""
    local URL=""
    local VARIANT=""

    [ "${ARCH}" == "arm64" ] && FEDORA_ARCH="aarch64"

    case ${EDITION} in
        Server|Kinoite|Onyx|Silverblue|Sericea|Workstation|KDE) VARIANT="${EDITION}";;
        *) VARIANT="Spins";;
    esac
    # Handle KDE as a proper edition from 42 but a spin prior to 42
    # Stripping eventual _Beta suffix from the RELEASE variable in the check
    if [[ "${VARIANT}" == "KDE" && "${RELEASE/_Beta/}" -lt 42 ]]; then
        VARIANT="Spins"
    fi
    # The naming of 41 Beta with a space is problematic so we replaced it with an underscore
    # but we need to convert it back to a space for the URL search in the JSON
    #shellcheck disable=SC2086
    # if RELEASE contains an underscore, replace it with a space
    if [[ "${RELEASE}" == *"_"* ]]; then
        RELEASE="${RELEASE/_/ }"
    fi

    # shellcheck disable=SC2086
    # Fedora may promote variants from Spins to Editions, in which case we want to accept either "Spins" or the specific edition name to preserve backwards compatibility
    # For example, Fedora 42 KDE is now an edition, while previous releases are spins
    JSON=$(web_pipe "https://getfedora.org/releases.json" | jq '.[] | select((.variant=="'"${VARIANT}"'" or .variant=="'"${EDITION}"'") and .subvariant=="'"${EDITION}"'" and .arch=="'"${FEDORA_ARCH}"'" and .version=="'"${RELEASE}"'" and (.link | endswith(".iso")))')
    URL=$(echo "${JSON}" | jq -r '.link' | head -n1)
    HASH=$(echo "${JSON}" | jq -r '.sha256' | head -n1)
    echo "${URL} ${HASH}"
}


function get_freebsd() {
    local HASH=""
    local ISO="FreeBSD-${RELEASE}-RELEASE-amd64-${EDITION}.iso"
    local URL="https://download.freebsd.org/ftp/releases/amd64/amd64/ISO-IMAGES/${RELEASE}"
    HASH=$(web_pipe "${URL}/CHECKSUM.SHA256-FreeBSD-${RELEASE}-RELEASE-amd64" | grep "${ISO}" | grep -v ".xz" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_freedos() {
    local HASH=""
    local ISO=""
    local BASE_URL="https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/distributions/${RELEASE}"
    local URL=""
    case ${RELEASE} in
        1.2) URL="${BASE_URL}/official"
             ISO="FD12CD.iso"
             HASH=$(web_pipe "${URL}/FD12.sha" | grep "${ISO}" | cut -d' ' -f1);;
        1.3) URL="${BASE_URL}/official"
             ISO="FD13-LiveCD.zip"
             HASH=$(web_pipe "${URL}/verify.txt" | grep -A 8 "sha256sum" | grep "${ISO}" | cut -d' ' -f1);;
        1.4) URL="${BASE_URL}"
             ISO="FD14-LiveCD.zip"
             HASH=$(web_pipe "${URL}/verify.txt" | grep -A 8 "sha256sum" | grep "${ISO}" | cut -d' ' -f1);;
    esac
    echo "${URL}/${ISO} ${HASH}"
}


function get_garuda() {
    local HASH=""
    local ISO=""
    local URL="https://iso.builds.garudalinux.org/iso/latest/garuda"
    ISO=${EDITION}/latest.iso
    HASH="$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f1)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_gentoo() {
    local HASH=""
    local ISO=""
    local URL="https://mirrors.kernel.org/gentoo/releases/amd64/autobuilds"
    case ${EDITION} in
        minimal) ISO=$(web_pipe "${URL}/${RELEASE}-iso.txt" | grep install | cut -d' ' -f1);;
        livegui) ISO=$(web_pipe "${URL}/${RELEASE}-iso.txt" | grep livegui | cut -d' ' -f1);;
    esac
    HASH=$(web_pipe "${URL}/${ISO}.DIGESTS" | grep -A 1 SHA512 | grep iso | grep -v CONTENTS | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_ghostbsd() {
    local ISO=""
    local URL="https://download.ghostbsd.org/releases/amd64/${RELEASE}"
    local HASH=""
    case ${EDITION} in
        mate) ISO="GhostBSD-${RELEASE}.iso";;
        xfce) ISO="GhostBSD-${RELEASE}-XFCE.iso";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | grep "${ISO}" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_gobolinux() {
    local HASH="8cd66e3e06c865c375134d099b8b483776ef16edc6d3cd617740c36f6ba53bd3"
    local ISO="GoboLinux-${RELEASE}-x86_64.iso"
    local URL="https://gobolinux.neonsys.org/${RELEASE}-ISO"
    echo "${URL}/${ISO} ${HASH}"
}


function get_gnomeos() {
    local HASH=""
    local ISO="gnome_os_installer_${RELEASE}.iso"
    local URL="https://download.gnome.org/gnomeos/${RELEASE}"
    case ${RELEASE} in
        nightly)
            ISO="gnome_os_installer.iso"
            URL="https://os.gnome.org/download/latest";;
        46.0) ISO="gnome_os_installer_46.iso";;
        3*) ISO="gnome_os_installer.iso";;
    esac
    # Process the URL redirections; required for GNOME
    ISO=$(web_redirect "${URL}/${ISO}")
    echo "${ISO} ${HASH}"
}


function get_guix() {
    local HASH=""
    local ISO="guix-system-install-${RELEASE}.x86_64-linux.iso"
    local URL="https://ftpmirror.gnu.org/gnu/guix/"
    echo "${URL}/${ISO} ${HASH}"
}


function get_haiku() {
    local ISO="haiku-${RELEASE}-${EDITION}-anyboot.iso"
    local URL="http://mirror.rit.edu/haiku/${RELEASE}"
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | grep "${ISO}" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_kali() {
    local HASH=""
    local ISO=""
    local URL="https://cdimage.kali.org/${RELEASE}"
    local SUFFIX="amd64.iso"
    case "${EDITION}" in
        purple)  SUFFIX="purple-amd64.iso";; 
        netinst) SUFFIX="netinst-amd64.iso";;
    esac
    ISO=$(web_pipe "${URL}/?C=M;O=D" | grep -o ">kali-linux-.*-installer-${SUFFIX}" | head -n 1 | cut -c 2-)
    HASH=$(web_pipe "${URL}/SHA256SUMS" | grep -v torrent | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_kdeneon() {
    local HASH=""
    local ISO=""
    local URL="https://files.kde.org/neon/images/${EDITION}/${RELEASE}/current"

    local CHKSUM_FILE_CONTENT=""
    CHKSUM_FILE_CONTENT=$(web_pipe "${URL}/neon-${RELEASE}-${EDITION}-current.sha256sum")
    IFS=' ' read -r HASH ISO <<< "$CHKSUM_FILE_CONTENT"

    echo "${URL}/${ISO} ${HASH}"
}


function get_kdelinux() {
    local HASH=""
    local ISO=""
    local URL="https://files.kde.org/kde-linux"
    ISO=$(curl -s "${URL}/?C=M;O=D" | grep -o 'kde-linux_[0-9]\{12\}\.iso' | head -n1)
    HASH=$(web_pipe "${URL}/SHA256SUMS" | grep "$ISO" | awk '{print $1}')
    echo "${URL}/${ISO} ${HASH}"
}


function get_kolibrios() {
    local HASH=""
    local ISO="latest-iso.7z"
    local URL="http://builds.kolibrios.org/${EDITION}"
    HASH=$(web_pipe "${URL}/sha256sums.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_templeos() {
    # TempleOS is frozen at 5.03 and publishes no checksums
    local ISO="TempleOS.ISO"
    local URL="https://templeos.org/Downloads"
    echo "${URL}/${ISO}"
}


function get_linuxlite() {
    local HASH=""
    local ISO="linux-lite-${RELEASE}-64bit.iso"
    local URL="https://sourceforge.net/projects/linux-lite/files/${RELEASE}"
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_linuxmint() {
    local HASH=""
    local ISO="linuxmint-${RELEASE}-${EDITION}-64bit.iso"
    local URL="https://mirrors.kernel.org/linuxmint/stable/${RELEASE}"
    HASH=$(web_pipe "${URL}/sha256sum.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_lmde() {
    local HASH=""
    local ISO="lmde-${RELEASE}-${EDITION}-64bit.iso"
    local URL="https://mirrors.kernel.org/linuxmint/debian"
    HASH=$(web_pipe "${URL}/sha256sum.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_maboxlinux() {
    local HASH=""
    local ISO=""
    local URL=""
    URL="https://sourceforge.net/projects/mabox-linux/files/${RELEASE}/download"
    URL="$(web_redirect "${URL}" | cut -d? -f1)"
    ISO="$(basename "${URL}")"
    HASH=$(web_pipe "https://repo.maboxlinux.org/iso/${ISO}.md5" | cut -d' ' -f1)
    echo "${URL} ${HASH}"
}


function generate_id() {
    local macRecoveryID=""
    local TYPE="${1}"
    local valid_chars=("0" "1" "2" "3" "4" "5" "6" "7" "8" "9" "A" "B" "C" "D" "E" "F")
    for ((i=0; i<TYPE; i++)); do
        macRecoveryID+="${valid_chars[$((RANDOM % 16))]}"
    done
    echo "${macRecoveryID}"
}


function get_macos() {
    local appleSession=""
    local info=""
    local downloadLink=""
    local downloadSession=""
    local chunkListLink=""
    local chunkListSession=""
    local BOARD_ID=""
    local CWD=""
    local CHECK=""
    local CHUNKCHECK=""
    local MLB="00000000000000000"
    local OS_TYPE="default"

    case ${RELEASE} in
        lion|10.7)
            BOARD_ID="Mac-2E6FAB96566FE58C"
            MLB="00000000000F25Y00";;
        mountainlion|10.8)
            BOARD_ID="Mac-7DF2A3B5E5D671ED"
            MLB="00000000000F65100";;
        mavericks|10.9)
            BOARD_ID="Mac-F60DEB81FF30ACF6"
            MLB="00000000000FNN100";;
        yosemite|10.10)
            BOARD_ID="Mac-E43C1C25D4880AD6"
            MLB="00000000000GDVW00";;
        elcapitan|10.11)
            BOARD_ID="Mac-FFE5EF870D7BA81A"
            MLB="00000000000GQRX00";;
        sierra|10.12)
            BOARD_ID="Mac-77F17D7DA9285301"
            MLB="00000000000J0DX00";;
        high-sierra|10.13)
            BOARD_ID="Mac-BE088AF8C5EB4FA2"
            MLB="00000000000J80300";;
        mojave|10.14)
            BOARD_ID="Mac-7BA5B2DFE22DDD8C"
            MLB="00000000000KXPG00";;
        catalina|10.15)
            BOARD_ID="Mac-00BE6ED71E35EB86";;
        big-sur|11)
            BOARD_ID="Mac-2BD1B31983FE1663";;
        monterey|12)
            BOARD_ID="Mac-B809C3757DA9BB8D"
            OS_TYPE="latest";;
        ventura|13)
            BOARD_ID="Mac-4B682C642B45593E"
            OS_TYPE="latest";;
        sonoma|14)
            BOARD_ID="Mac-827FAC58A8FDFA22";;
        sequoia|15)
            BOARD_ID="Mac-7BA5B2D9E42DDD94";;
        tahoe|26)
            BOARD_ID="Mac-CFF7D910A743CAAF"
            OS_TYPE="latest";;
        *) echo "ERROR! Unknown release: ${RELEASE}"
           releases_macos
           exit 1;;
    esac

    CWD="$(dirname "${0}")"
    if [ -x "${CWD}/../libexec/quickemu/chunkcheck.py" ]; then
        CHUNKCHECK="${CWD}/../libexec/quickemu/chunkcheck.py"
    elif [ -x "$(command -v chunkcheck)" ]; then
        CHUNKCHECK="$(command -v chunkcheck)"
    fi

    appleSession=$(curl --disable -v -H "Host: osrecovery.apple.com" \
                           -H "Connection: close" \
                           -A "InternetRecovery/1.0" https://osrecovery.apple.com/ 2>&1 | tr ';' '\n' | awk -F'session=|;' '{print $2}' | grep 1)
    info=$(curl --disable -s -X POST -H "Host: osrecovery.apple.com" \
                           -H "Connection: close" \
                           -A "InternetRecovery/1.0" \
                           -b "session=\"${appleSession}\"" \
                           -H "Content-Type: text/plain" \
                           -d $'cid='"$(generate_id 16)"$'\nsn='${MLB}$'\nbid='${BOARD_ID}$'\nk='"$(generate_id 64)"$'\nfg='"$(generate_id 64)"$'\nos='${OS_TYPE} \
                           https://osrecovery.apple.com/InstallationPayload/RecoveryImage | tr ' ' '\n')
    downloadLink=$(echo "$info" | grep 'oscdn' | grep 'dmg')
    downloadSession=$(echo "$info" | grep 'expires' | grep 'dmg')
    chunkListLink=$(echo "$info" | grep 'oscdn' | grep 'chunklist')
    chunkListSession=$(echo "$info" | grep 'expires' | grep 'chunklist')

    if [ "${OPERATION}" == "show" ]; then
        test_result "${OS}" "${RELEASE}" "" "${downloadLink}"
        exit 0
    elif [ "${OPERATION}" == "test" ]; then
        CHECK=$(web_check "${downloadLink}" --header "Host: oscdn.apple.com" --header "Connection: close" --header "User-Agent: InternetRecovery/1.0" --header "Cookie: AssetToken=${downloadSession}" && echo "PASS" || echo "FAIL")
        test_result "${OS}" "${RELEASE}" "" "${downloadLink}" "${CHECK}"
        exit 0
    elif [ "${OPERATION}" == "download" ]; then
        echo "Downloading macOS (${RELEASE^}) RecoveryImage"
        echo " - URL: ${downloadLink}"
        web_get "${downloadLink}" "${VM_PATH}" RecoveryImage.dmg --header "Host: oscdn.apple.com" --header "Connection: close" --header "User-Agent: InternetRecovery/1.0" --header "Cookie: AssetToken=${downloadSession}"
        web_get "${chunkListLink}" "${VM_PATH}" RecoveryImage.chunklist --header "Host: oscdn.apple.com" --header "Connection: close" --header "User-Agent: InternetRecovery/1.0" --header "Cookie: AssetToken=${chunkListSession}"
        VM_PATH="$(pwd)"
    else
        if [ ! -e "${VM_PATH}/RecoveryImage.chunklist" ]; then
            echo "Downloading macOS (${RELEASE^}) RecoveryImage"
            echo " - URL: ${downloadLink}"
            web_get "${downloadLink}" "${VM_PATH}" RecoveryImage.dmg --header "Host: oscdn.apple.com" --header "Connection: close" --header "User-Agent: InternetRecovery/1.0" --header "Cookie: AssetToken=${downloadSession}"
            web_get "${chunkListLink}" "${VM_PATH}" RecoveryImage.chunklist --header "Host: oscdn.apple.com" --header "Connection: close" --header "User-Agent: InternetRecovery/1.0" --header "Cookie: AssetToken=${chunkListSession}"
            if ! "${CHUNKCHECK}" "${VM_PATH}" 2> /dev/null; then
                echo " - WARNING! Verification failed, continuing anyway"
            else
                echo " - Verification passed"
            fi

            if [ -e "${VM_PATH}/RecoveryImage.dmg" ] && [ ! -e "${VM_PATH}/RecoveryImage.img" ]; then
                require_qemu_img
                echo " - Converting RecoveryImage.dmg"
                ${QEMU_IMG} convert "${VM_PATH}/RecoveryImage.dmg" -O raw "${VM_PATH}/RecoveryImage.img" 2>/dev/null
            fi
            rm "${VM_PATH}/RecoveryImage.dmg" "${VM_PATH}/RecoveryImage.chunklist"
            echo " - RecoveryImage.img is ready."
        fi

        echo "Downloading OpenCore & UEFI firmware"
        # Pin to last known-good commit; OVMF_CODE.fd was removed from master on 26 Jan 2026
        local OSX_KVM_COMMIT="da4b23b5e92c5b939568700034367e8b7649fe90"
        web_get "https://github.com/kholia/OSX-KVM/raw/${OSX_KVM_COMMIT}/OpenCore/OpenCore.qcow2" "${VM_PATH}"
        web_get "https://github.com/kholia/OSX-KVM/raw/${OSX_KVM_COMMIT}/OVMF_CODE.fd" "${VM_PATH}"
        if [ ! -e "${VM_PATH}/OVMF_VARS-1920x1080.fd" ]; then
            web_get "https://github.com/kholia/OSX-KVM/raw/${OSX_KVM_COMMIT}/OVMF_VARS-1920x1080.fd" "${VM_PATH}"
        fi
    fi
    make_vm_config RecoveryImage.img
}


function get_mageia() {
    local HASH=""
    local ISO=""
    ISO=$(web_pipe https://www.mageia.org/en/downloads/get/?q="Mageia-${RELEASE}-Live-${EDITION}-x86_64.iso" | grep 'click here'| grep -o 'href=.*\.iso'|cut -d\" -f2)
    HASH=$(web_pipe "${ISO}.sha512" | cut -d' ' -f1)
    echo "${ISO} ${HASH}"
}


function get_manjaro() {
    local HASH=""
    local ISO=""
    local MANIFEST=""
    local URL=""
    local TYPE="official"
    MANIFEST="$(web_pipe https://gitlab.manjaro.org/web/iso-info/-/raw/master/file-info.json)"
    case "${RELEASE}" in
        sway)
            MANIFEST="$(web_pipe https://mirror.manjaro-sway.download/manjaro-sway/release.json)"
            TYPE="sway"
            ;;
        cinnamon|i3) TYPE="community";;
    esac

    if [ "${EDITION}" == "minimal" ] && [ "${TYPE}" != "sway" ]; then
        EDITION=".minimal"
    else
        EDITION=""
    fi

    if [ "${RELEASE}" == "sway" ]; then
        URL=$(echo "${MANIFEST}" | jq -r '.[] | select(.name|test("^manjaro-sway-.*[.]iso$")) | .url')
    else
        URL="$(echo "${MANIFEST}" | jq -r ."${TYPE}.${RELEASE}${EDITION}".image)"
    fi
    HASH=$(web_pipe "${URL}.sha512" | cut -d' ' -f1)
    echo "${URL} ${HASH}"
}


function get_mxlinux() {
    local HASH=""
    local ISO=""
    local URL="https://sourceforge.net/projects/mx-linux/files/Final/${EDITION}"
    case ${EDITION} in
        Xfce) ISO="MX-${RELEASE}_Xfce_x64.iso";;
        KDE) ISO="MX-${RELEASE}_KDE_x64.iso";;
        Fluxbox) ISO="MX-${RELEASE}_fluxbox_x64.iso";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_netboot() {
    local HASH=""
    local ISO="netboot.xyz.iso"
    local URL="https://boot.netboot.xyz/ipxe"
    HASH=$(web_pipe "${URL}/netboot.xyz-sha256-checksums.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_netbsd() {
    local HASH=""
    local ISO="NetBSD-${RELEASE}-amd64.iso"
    local URL="https://cdn.netbsd.org/pub/NetBSD/NetBSD-${RELEASE}/images"
    HASH=$(web_pipe "${URL}/MD5" | grep "${ISO}" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_nitrux() {
    local HASH=""
    local URLBASE=""
    local URL=""
    local ISO=""
    URLBASE="https://sourceforge.net/projects/nitruxos/files/Release"
    URL="${URLBASE}/ISO"
    ISO=$(web_pipe 'https://sourceforge.net/projects/nitruxos/rss?path=/Release/ISO' | grep '.iso' | head -n 1 | cut -d']' -f1 | cut -d '/' -f4)
    HASH=$(web_pipe "${URLBASE}/MD5/${ISONAME}.md5sum" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_nixos() {
    local HASH=""
    local ISO="latest-nixos-${EDITION}-x86_64-linux.iso"
    local URL="https://channels.nixos.org/nixos-${RELEASE}"
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_nwg-shell() {
    local HASH=""
    local ISO="nwg-live-${RELEASE}-x86_64.iso"
    local URL="https://sourceforge.net/projects/nwg-iso/files"
    HASH="$(web_pipe "https://sourceforge.net/projects/nwg-iso/rss?path=/" | grep "${ISO}" | cut -d'>' -f3 | cut -d'<' -f1 | tail -n 1)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_omarchy() {
    local HASH="9224fab3720560f771969a99a499e5f7e0f8e2d6a0681d872d52f05fb5003da4"
    local ISO="omarchy-${RELEASE}.iso"
    local URL="https://iso.omarchy.org"
    echo "${URL}/${ISO} ${HASH}"
}


function get_openfyde() {
    local HASH=""
    local ISO="amd64-openfyde-${RELEASE}.bin.xz"
    local URL="https://github.com/openfyde/overlay-amd64-openfyde/releases/download/${RELEASE}"
    echo "${URL}/${ISO} ${HASH}"
}


function get_openbsd() {
    local HASH=""
    local ISO="install${RELEASE//\./}.iso"
    local URL="https://mirror.leaseweb.com/pub/OpenBSD/${RELEASE}/amd64"
    HASH=$(web_pipe "${URL}/SHA256" | grep "${ISO}" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_openindiana() {
    local HASH=""
    local ISO=""
    local URL=""
    URL="https://dlc.openindiana.org/isos/hipster/${RELEASE}"
    ISO="OI-hipster-${EDITION}-${RELEASE}.iso"
    HASH=$(web_pipe "${URL}/${ISO}.sha256" |cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_opensuse() {
    local HASH=""
    local ISO=""
    local URL=""
    if [ "${RELEASE}" == "tumbleweed" ]; then
        ISO="openSUSE-Tumbleweed-DVD-x86_64-Current.iso"
        URL="https://download.opensuse.org/tumbleweed/iso"
    elif [ "${RELEASE}" == "microos" ]; then
        ISO="openSUSE-MicroOS-DVD-x86_64-Current.iso"
        URL="https://download.opensuse.org/tumbleweed/iso"
    elif [ "${RELEASE}" == 15.0 ] || [ "${RELEASE}" == 15.1 ]; then
        ISO="openSUSE-Leap-${RELEASE}-DVD-x86_64.iso"
        URL="https://download.opensuse.org/distribution/leap/${RELEASE}/iso"
    elif [ "${RELEASE}" == 16.0 ]; then
        ISO="Leap-${RELEASE}-offline-installer-x86_64.install.iso"
        URL="https://download.opensuse.org/distribution/leap/${RELEASE}/offline"
    else
        ISO="openSUSE-Leap-${RELEASE}-DVD-x86_64-Current.iso"
        URL="https://download.opensuse.org/distribution/leap/${RELEASE}/iso"
    fi
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | awk '{if(NR==4) print $0}' | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_oraclelinux() {
    local HASH=""
    local ISO=""
    local VER_MAJ=${RELEASE::1}
    local VER_MIN=${RELEASE:2:1}
    local URL="https://yum.oracle.com/ISOS/OracleLinux/OL${VER_MAJ}/u${VER_MIN}/x86_64"
    case ${VER_MAJ} in
        7) ISO="OracleLinux-R${VER_MAJ}-U${VER_MIN}-Server-x86_64-dvd.iso";;
        *) ISO="OracleLinux-R${VER_MAJ}-U${VER_MIN}-x86_64-dvd.iso";;
    esac
    HASH=$(web_pipe "https://linux.oracle.com/security/gpg/checksum/OracleLinux-R${VER_MAJ}-U${VER_MIN}-Server-x86_64.checksum" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_parrotsec() {
    local HASH=""
    local ISO=""
    local URL=""
    ISO="Parrot-${EDITION}-${RELEASE}_amd64.iso"
    URL="https://download.parrot.sh/parrot/iso/${RELEASE}"
    HASH="$(web_pipe "${URL}/signed-hashes.txt" | grep "${ISO}" | cut -d' ' -f1)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_pclinuxos() {
    local URL="https://ftp.fau.de/pclinuxos/pclinuxos/iso"
    local HASH=""
    local ISO=""
    # Find the latest ISO for this edition dynamically
    ISO=$(web_pipe "${URL}/" | grep -o "pclinuxos64-${EDITION}-[0-9.]\+\.iso" | sort -Vr | head -n 1)
    if [ -z "${ISO}" ]; then
        echo ""
        return
    fi
    HASH=$(web_pipe "${URL}/${ISO}.md5sum" | head -c 32)
    echo "${URL}/${ISO} ${HASH}"
}


function get_peppermint() {
    local HASH=""
    local ISO=""
    local URL="https://sourceforge.net/projects/peppermintos/files/isos"
    case ${EDITION} in
        devuan-xfce)
            ISO="PeppermintOS-devuan_64_xfce.iso"
            URL="${URL}/XFCE";;
        debian-xfce)
            ISO="PeppermintOS-Debian-64.iso"
            URL="${URL}/XFCE";;
        devuan-gnome)
            ISO="PeppermintOS-devuan_64_gfb.iso"
            URL="${URL}/Gnome_FlashBack";;
        debian-gnome)
            ISO="PeppermintOS-Debian_64_gfb.iso"
            URL="${URL}/Gnome_FlashBack";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}-sha512.checksum" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_popos() {
    local HASH=""
    local ISO=""
    local URL=""
    URL=$(web_pipe "https://api.pop-os.org/builds/${RELEASE}/${EDITION}" | jq -r .url)
    HASH=$(web_pipe "https://api.pop-os.org/builds/${RELEASE}/${EDITION}" | jq -r .sha_sum)
    echo "${URL} ${HASH}"
}


function get_porteus() {
    local HASH=""
    local ISO=""
    local URL=""
    edition="${EDITION~~}"
    ISO="Porteus-${edition}-v${RELEASE}-x86_64.iso"
    URL="https://mirrors.dotsrc.org/porteus/x86_64/Porteus-v${RELEASE}"
    HASH=$(web_pipe "${URL}/sha256sums.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_primtux() {
    local HASH=""
    local URL=""
    local ISO=""
    ISO="PrimTux${RELEASE}-amd64-${EDITION}.iso"
    URL="https://sourceforge.net/projects/primtux/files/Distribution"
    HASH=$(web_pipe "${URL}/${ISO}.md5" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_proxmox-ve() {
    local HASH=""
    local ISO=""
    local URL=""
    ISO="proxmox-ve_${RELEASE}.iso"
    URL="https://enterprise.proxmox.com/iso"
    HASH=$(web_pipe "${URL}/SHA256SUMS" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"

}


function get_pureos() {
    local HASH=""
    local ISO=""
    local URL=""
    local PureName=
    PureName="$(web_pipe "https://www.pureos.net/download/" | grep -m 1 "downloads.puri" | cut -d '/' -f 4)"
    local PureDate=
    PureDate="$(web_pipe "https://www.pureos.net/download/" | grep -m 1 "downloads.puri" | cut -d '/' -f 6)"
    local PureDateSquashed="${PureDate//'-'/}"
    edition="${EDITION,,}"
    URL="https://downloads.puri.sm/${PureName}/${edition}/${PureDate}"
    ISO="pureos-${RELEASE}-${edition}-live-${PureDateSquashed}_amd64.iso"
    local IsoTrimmed=
    IsoTrimmed="${ISO%.*}"
    HASH="$(web_pipe "${URL}/${IsoTrimmed}.checksums_sha256.txt" | grep -m 1 '.iso' | cut -d '.' -f 1)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_qubes() {
    local HASH=""
    local ISO="Qubes-R${RELEASE}-x86_64.iso"
    local URL="https://ftp.qubes-os.org/iso"
    HASH=$(web_pipe "${URL}/${ISO}.DIGESTS" | grep -E '^[0-9a-f]{64} ' | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_redoxos() {
    local HASH=""
    local ISO=""
    local LINE=""
    local URL="https://static.redox-os.org/releases/${RELEASE}/x86_64"
    LINE=$(web_pipe "${URL}/SHA256SUM" | grep "redox_${EDITION}_.*_livedisk.iso.zst" | tail -1)
    ISO=$(grep -oE 'redox_[^ ]*livedisk\.iso\.zst' <<<"${LINE}" | head -1)
    HASH=$(cut -d' ' -f1 <<<"${LINE}")
    echo "${URL}/${ISO} ${HASH}"
}


function get_reactos() {
    local HASH=""
    local URL=""
    URL="$(web_redirect "https://sourceforge.net/projects/reactos/files/latest/download")"
    echo "${URL} ${HASH}"
}


function get_rebornos() {
    local HASH=""
    local ISO=""
    ISO=$(web_pipe "https://meta.cdn.soulharsh007.dev/RebornOS-ISO?format=json" | jq -r ".url")
    HASH=$(web_pipe "https://meta.cdn.soulharsh007.dev/RebornOS-ISO?format=json" | jq -r ".md5")
    echo "${ISO} ${HASH}"
}


function get_rockylinux() {
    if { [[ "${RELEASE}" =~ ^8. ]] || [[ "${RELEASE}" =~ ^10. ]]; } && [[ "${EDITION}" == "dvd" ]]; then
        EDITION="dvd1"
    fi
    local HASH=""
    local ISO="Rocky-${RELEASE}-x86_64-${EDITION}.iso"
    local URL="https://dl.rockylinux.org/vault/rocky/${RELEASE}/isos/x86_64"
    HASH=$(web_pipe "${URL}/CHECKSUM" | grep "SHA256" | grep "${ISO})" | cut -d' ' -f4)
    echo "${URL}/${ISO} ${HASH}"
}


function get_siduction() {
    local HASH=""
    local DATE=""
    local ISO=""
    local NAME=""
    local URL=""
    NAME=$(web_pipe "https://mirror.math.princeton.edu/pub/siduction/iso/" | grep folder | cut -d'"' -f8 | tr -d '/')
    URL="https://mirrors.dotsrc.org/siduction/iso/${NAME}/${EDITION}"
    DATE=$(web_pipe "${URL}"| grep .iso.md5 | cut -d'-' -f6 | cut -d'.' -f1)
    HASH=$(web_pipe "${URL}/${ISO}.md5" | cut -d' ' -f1)
    VERSION=$(web_pipe "${URL}"| grep .iso.md5 | cut -d'-' -f2)
    ISO="siduction-${VERSION}-${NAME}-${EDITION}-amd64-${DATE}.iso"
    echo "${URL}/${ISO} ${HASH}"
}


function get_slackware() {
    local HASH=""
    local ISO="slackware64-${RELEASE}-install-dvd.iso"
    local URL="https://slackware.nl/slackware/slackware-iso/slackware64-${RELEASE}-iso"
    HASH=$(web_pipe "${URL}/${ISO}.md5" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_slax() {
    local HASH=""
    local ISO=""
    local URL=""
    case ${EDITION} in
        debian)
            URL="https://ftp.fi.muni.cz/pub/linux/slax/Slax-12.x"
            ISO=$(web_pipe "${URL}/md5.txt" | grep '64bit-' | cut -d' ' -f3 | tail -n1);;
        slackware)
            URL="https://ftp.fi.muni.cz/pub/linux/slax/Slax-15.x"
            ISO=$(web_pipe "${URL}/md5.txt" | grep '64bit-' | cut -d' ' -f3 | tail -n1);;
    esac
    HASH=$(web_pipe "${URL}/md5.txt" | grep '64bit-' | cut -d' ' -f1 | tail -n1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_slint() {
    local HASH=""
    local MAJ_VER=""
    local ISO="slint64-${RELEASE}.iso"
    MAJ_VER="$(echo "${RELEASE}" | cut -d'-' -f 1)"
    local URL="https://slackware.uk/slint/x86_64/slint-${MAJ_VER}/iso"
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f4)
    echo "${URL}/${ISO}" "${HASH}"
}


function get_slitaz() {
    local HASH=""
    local ISO="slitaz-rolling-${RELEASE}"
    local URL="http://mirror.slitaz.org/iso/rolling"
    HASH=$(web_pipe "${URL}/${ISO}.md5" | cut -d' ' -f1)
    echo "${URL}/${ISO}.iso ${HASH}"
}


function get_solus() {
    local HASH=""
    local ISO=""
    local URL="https://downloads.getsol.us/isos/${RELEASE}"
    # Try standard naming first, then beta naming for Xfce (older releases had Xfce as beta)
    ISO="Solus-${EDITION}-Release-${RELEASE}.iso"
    if ! web_check "${URL}/${ISO}"; then
        # Try uppercase XFCE-Beta naming for older releases
        local EDITION_UPPER="${EDITION^^}"
        ISO="Solus-${EDITION_UPPER}-Beta-Release-${RELEASE}.iso"
    fi
    HASH=$(web_pipe "${URL}/${ISO}.sha256sum" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_sparkylinux() {
    local HASH=""
    local ISO=""
    local URL=""
    ISO="sparkylinux-${RELEASE}-x86_64-${EDITION}.iso"
    case ${EDITION} in
        minimalcli) URL="https://sourceforge.net/projects/sparkylinux/files/cli";;
        minimalgui) URL="https://sourceforge.net/projects/sparkylinux/files/base";;
        *) URL="https://sourceforge.net/projects/sparkylinux/files/${EDITION}";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}.allsums.txt" | head -n 2 | grep 'iso' | cut -d' ' -f1)
    echo "${URL}/${ISO}" "${HASH}"
}


function get_spirallinux() {
    local HASH=""
    local ISO="SpiralLinux_${EDITION}_12.231005_x86-64.iso"
    local URL="https://sourceforge.net/projects/spirallinux/files/12.231005"
    HASH=$(web_pipe 'https://sourceforge.net/projects/spirallinux/rss?path=/' | grep "${ISO}" | grep 'md5' | cut -d'<' -f3 | cut -d'>' -f2)
    echo "${URL}/${ISO}" "${HASH}"
}


function get_tails() {
    local JSON=""
    local HASH=""
    local URL=""
    JSON="$(web_pipe "https://tails.boum.org/install/v2/Tails/amd64/${RELEASE}/latest.json")"
    URL=$(echo "${JSON}" | jq -r '.installations[0]."installation-paths"[]|select(.type=="iso")|."target-files"[0].url')
    HASH=$(echo "${JSON}" | jq -r '.installations[0]."installation-paths"[]|select(.type=="iso")|."target-files"[0].sha256')
    echo "${URL} ${HASH}"
}


function get_tinycore() {
    local ARCH="x86"
    local HASH=""
    local ISO="${EDITION}-${RELEASE}.0.iso"
    case "${EDITION}" in
        *Pure*) ARCH+="_64";;
    esac
    local URL="http://www.tinycorelinux.net/${RELEASE}.x/${ARCH}/release"
    HASH=$(web_pipe "${URL}/${ISO}.md5.txt" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_trisquel() {
    local HASH=""
    local ISO=""
    local URL="https://mirrors.ocf.berkeley.edu/trisquel-images"
    case ${EDITION} in
        mate) ISO="trisquel_${RELEASE}_amd64.iso";;
        lxde) ISO="trisquel-mini_${RELEASE}_amd64.iso";;
        kde) ISO="triskel_${RELEASE}_amd64.iso";;
        sugar) ISO="trisquel-sugar_${RELEASE}_amd64.iso";;
    esac
    HASH=$(web_pipe "${URL}/${ISO}.sha1" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_tuxedo-os() {
    local HASH=""
    local ISO=""
    local URL="https://os.tuxedocomputers.com"
    ISO="$(web_pipe "https://os.tuxedocomputers.com/" | grep -m 1 current.iso | cut -d '=' -f 4 | cut -d '"' -f 2)"
    HASH="$(web_pipe "https://os.tuxedocomputers.com/checksums/${ISO}.sha256" | cut -d ' ' -f 1)"
    echo "${URL}/${ISO} ${HASH}"
}


function get_ubuntu-server() {
    local DATA=""
    local HASH=""
    local ISO=""
    local NAME="live-server"
    local UBUNTU_ARCH="${ARCH}"
    local URL=""

    if [[ "${RELEASE}" == "daily"* ]]; then
        # Daily images live in per-codename directories since Ubuntu
        # restructured cdimage.ubuntu.com
        local DAILY_CODENAME=""
        DAILY_CODENAME="$(ubuntu_daily_codename "${OS}")"
        if [ -z "${DAILY_CODENAME}" ]; then
            echo "$(pretty_name "${OS}") ${RELEASE} is currently unavailable. Please select other OS/Release combination"
            exit 1
        fi
        URL="https://cdimage.ubuntu.com/${OS}/${DAILY_CODENAME}/daily-live/current"
    elif [ "${UBUNTU_ARCH}" == "arm64" ]; then
        # ARM64 ISOs are hosted on cdimage.ubuntu.com
        URL="https://cdimage.ubuntu.com/releases/${RELEASE}/release"
    else
        URL="https://releases.ubuntu.com/${RELEASE}"
    fi

    case "${RELEASE}" in
        14*|16*) NAME="server";;
    esac

    if web_check "${URL}/SHA256SUMS"; then
        DATA=$(web_pipe "${URL}/SHA256SUMS" | grep "${NAME}" | grep "${UBUNTU_ARCH}" | grep iso | tail -n 1 )
        ISO=$(cut -d'*' -f2 <<<"${DATA}")
        HASH=$(cut -d' ' -f1 <<<"${DATA}")
    else
        DATA=$(web_pipe "${URL}/MD5SUMS" | grep "${NAME}" | grep "${UBUNTU_ARCH}" | grep iso | tail -n 1 )
        ISO=$(cut -d' ' -f3 <<<"${DATA}")
        HASH=$(cut -d' ' -f1 <<<"${DATA}")
    fi
    if [[ "${RELEASE}" == "daily"* ]] || [ "${RELEASE}" == "dvd" ]; then
        zsync_get "${URL}/${ISO}" "${VM_PATH}" "${OS}-devel.iso"
        make_vm_config "${OS}-devel.iso"
    else
        web_get "${URL}/${ISO}" "${VM_PATH}"
        check_hash "${ISO}" "${HASH}"
        make_vm_config "${ISO}"
    fi
}


function get_ubuntu() {
    local DATA=""
    local DAILY_CODENAME=""
    local HASH=""
    local ISO=""
    local UBUNTU_ARCH="${ARCH}"
    local URL=""

    # Validate architecture support now that RELEASE is known
    if ! is_arch_supported "${OS}" "${UBUNTU_ARCH}"; then
        if [ "${OPERATION}" == "test" ] || [ "${OPERATION}" == "show" ]; then
            test_result "${OS}" "${RELEASE}" "" "" "SKIP" "(not available for ${UBUNTU_ARCH})"
            exit 0
        else
            echo "ERROR! $(pretty_name "${OS}") ${RELEASE} is not available for ${UBUNTU_ARCH} architecture."
            exit 1
        fi
    fi

    if [[ "${RELEASE}" == "daily"* ]] || [ "${RELEASE}" == "dvd" ]; then
        # Daily images live in per-codename directories since Ubuntu
        # restructured cdimage.ubuntu.com
        DAILY_CODENAME="$(ubuntu_daily_codename "${OS}")"
        if [ -z "${DAILY_CODENAME}" ]; then
            echo "$(pretty_name "${OS}") ${RELEASE} is currently unavailable. Please select other OS/Release combination"
            exit 1
        fi
        URL="https://cdimage.ubuntu.com/${OS}/${DAILY_CODENAME}/daily-live/current"
        VM_PATH="${OS}-${RELEASE}$(arch_suffix)"
    elif [ "${OS}" == "ubuntu" ] && [ "${UBUNTU_ARCH}" == "arm64" ]; then
        # ARM64 desktop ISOs are hosted on cdimage.ubuntu.com
        URL="https://cdimage.ubuntu.com/releases/${RELEASE}/release"
    elif [ "${OS}" == "ubuntu" ]; then
        URL="https://releases.ubuntu.com/${RELEASE}"
    else
        URL="https://cdimage.ubuntu.com/${OS}/releases/${RELEASE}/release"
    fi
    if web_check "${URL}/SHA256SUMS"; then
        DATA=$(web_pipe "${URL}/SHA256SUMS" | grep 'desktop\|dvd\|install' | grep "${UBUNTU_ARCH}" | grep iso | grep -v "+mac" | tail -n 1 )
        ISO=$(cut -d'*' -f2 <<<"${DATA}" | sed '1q;d')
        HASH=$(cut -d' ' -f1 <<<"${DATA}" | sed '1q;d')
    else
        DATA=$(web_pipe "${URL}/MD5SUMS" | grep 'desktop\|dvd\|install' | grep "${UBUNTU_ARCH}" | grep iso | grep -v "+mac" | tail -n 1 )
        ISO=$(cut -d'*' -f2 <<<"${DATA}")
        HASH=$(cut -d' ' -f1 <<<"${DATA}")
    fi
    if [ -z "${ISO}" ] || [ -z "${HASH}" ]; then
        echo "$(pretty_name "${OS}") ${RELEASE} is currently unavailable. Please select other OS/Release combination"
        exit 1
    fi
    if [[ "${RELEASE}" == "daily"* ]] || [ "${RELEASE}" == "dvd" ]; then
        zsync_get "${URL}/${ISO}" "${VM_PATH}" "${OS}-devel.iso"
        make_vm_config "${OS}-devel.iso"
    else
        web_get "${URL}/${ISO}" "${VM_PATH}"
        check_hash "${ISO}" "${HASH}"
        make_vm_config "${ISO}"
    fi
}


function get_vanillaos() {
    local HASH=""
    local HASH_URL=""
    local ISO=""
    ISO=$(web_pipe "https://api.github.com/repos/Vanilla-OS/live-iso/releases" | grep 'download_url' | grep "${RELEASE}" | head -n 1 | cut -d'"' -f4)
    HASH_URL="${ISO//.iso/.sha256.txt}"
    HASH=$(web_pipe "${HASH_URL}" | cut -d' ' -f1)
    echo "${ISO} ${HASH}"
}


function get_void() {
    local DATE=""
    local HASH=""
    local ISO=""
    local URL="https://repo-default.voidlinux.org/live"
    case ${EDITION} in
        glibc) ISO="void-live-x86_64-${RELEASE}-base.iso";;
        musl) ISO="void-live-x86_64-musl-${RELEASE}-base.iso";;
        xfce-glibc) ISO="void-live-x86_64-${RELEASE}-xfce.iso";;
        xfce-musl) ISO="void-live-x86_64-musl-${RELEASE}-xfce.iso";;
    esac
    HASH="$(web_pipe "${URL}/sha256sum.txt" | grep "${ISO}" | cut -d' ' -f4)"
    echo "${URL}/${RELEASE}/${ISO} ${HASH}"
}


function get_zorin() {
    local HASH=""
    local ISO=""
    local URL="https://plug-mirror.rcac.purdue.edu/zorin-iso/${RELEASE}"
    local EDITION_NAME=""
    # Convert edition code to ISO naming
    case ${EDITION} in
        core64) EDITION_NAME="Core-64-bit";;
        lite64) EDITION_NAME="Lite-64-bit";;
        education64) EDITION_NAME="Education-64-bit";;
    esac
    # Find the latest revision (r2, r1, or base)
    ISO=$(web_pipe "${URL}/" | grep -o "Zorin-OS-[0-9.]*-${EDITION_NAME}[^\"]*\.iso" | grep -v Beta | sort -Vr | head -n 1)
    if [ -z "${ISO}" ]; then
        echo ""
        return
    fi
    HASH=$(web_pipe "${URL}/SHA256SUMS.txt" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}


function get_windows() {
    if [ "${OS}" == "windows-server" ]; then
        download_windows_server "windows-server-${RELEASE}"
    else
        download_windows_workstation "${RELEASE}"
    fi

    if [ "${OPERATION}" == "download" ]; then
        exit 0
    fi

    echo "Downloading VirtIO drivers..."
    web_get "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso" "${VM_PATH}"

    rm -f "${VM_PATH}/unattended.iso"
    case ${RELEASE} in
        10|11)
            unattended_windows "${VM_PATH}" "w${RELEASE}"
            ;;
    esac

    if [ -n "${FILE_NAME}" ]; then
        make_vm_config "${FILE_NAME}" "virtio-win.iso"
    else
        make_vm_config "windows-${RELEASE}.iso" "virtio-win.iso"
    fi
}

function get_nobara() {
    local HASH="" ISO="" URL="https://nobaraproject.org/download-nobara/"
    ISO=$(web_pipe "${URL}" | grep -oE "Nobara-${RELEASE}-${EDITION}-[0-9-]+\.iso" | sort -V | tail -1)
    echo "https://nobara-images.nobaraproject.org/${ISO} ${HASH}"
}

function get_opnsense() {
    local HASH="" ISO="OPNsense-${RELEASE}-dvd-amd64.iso.bz2" URL="https://mirror.fra10.de.leaseweb.net/opnsense/releases/${RELEASE}"
    HASH=$(web_pipe "${URL}/OPNsense-${RELEASE}-checksums-amd64.sha256" | grep "${ISO}" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}

function get_vyos() {
    local HASH="" ISO="vyos-${RELEASE}-generic-amd64.iso" URL="https://github.com/vyos/vyos-nightly-build/releases/download/${RELEASE}"
    echo "${URL}/${ISO} ${HASH}"
}

function get_blackarch() {
    local HASH="" ISO="blackarch-linux-${EDITION}-${RELEASE}-x86_64.iso" URL="https://ftp.halifax.rwth-aachen.de/blackarch/iso"
    echo "${URL}/${ISO} ${HASH}"
}

function get_blissos() {
    local HASH="" ISO="" URL="https://sourceforge.net/projects/blissos-x86/rss?path=/"
    ISO=$(web_pipe "${URL}" | grep -oE "Official/BlissOS[0-9]+/Gapps/Generic/Bliss-v${RELEASE}-[^\"<]*\.iso" | sort -V | tail -1)
    echo "https://downloads.sourceforge.net/project/blissos-x86/${ISO} ${HASH}"
}

function get_kaos() {
    local HASH="" ISO="KaOS-${RELEASE}-x86_64.iso" URL="https://downloads.sourceforge.net/project/kaosx/ISO"
    echo "${URL}/${ISO} ${HASH}"
}

function get_midnightbsd() {
    local HASH="" ISO="MidnightBSD-${RELEASE}--amd64-disc1.iso" URL="https://discovery.midnightbsd.org/ftp/releases/amd64/ISO-IMAGES/${RELEASE}"
    HASH=$(web_pipe "${URL}/CHECKSUM.SHA256" | grep "${ISO}" | awk '{print $NF}')
    echo "${URL}/${ISO} ${HASH}"
}

function get_openmandriva() {
    local HASH="" ISO="" SUFFIX="" URL="https://sourceforge.net/projects/openmandriva/rss?path=/release/ROME"
    case "${EDITION}" in
        plasma)         SUFFIX="plasma6x11";;
        plasma-wayland) SUFFIX="plasma6wayland";;
        gnome)          SUFFIX="gnome3";;
        lxqt)           SUFFIX="lxqt";;
    esac
    ISO=$(web_pipe "${URL}" | grep -oE "files/release/ROME/[^\"<]*-${SUFFIX}\.x86_64\.iso" | sort -V | tail -1)
    ISO="${ISO//%28/(}"; ISO="${ISO//%29/)}"; ISO="${ISO#files/}"
    HASH=$(web_pipe "https://downloads.sourceforge.net/project/openmandriva/${ISO}.sha256sum" | cut -d' ' -f1)
    echo "https://downloads.sourceforge.net/project/openmandriva/${ISO} ${HASH}"
}

function get_openmediavault() {
    local HASH="" ISO="openmediavault_${RELEASE}-amd64.iso" URL="https://downloads.sourceforge.net/project/openmediavault/iso/${RELEASE}"
    HASH=$(web_pipe "${URL}/${ISO}.sha256" | cut -d' ' -f1)
    echo "${URL}/${ISO} ${HASH}"
}

function get_q4os() {
    local HASH="" ISO="" SUFFIX="" URL="https://sourceforge.net/projects/q4os/rss?path=/stable"
    case "${EDITION}" in
        kde)     SUFFIX="x64\\.r";;
        trinity) SUFFIX="x64-tde\\.r";;
    esac
    ISO=$(web_pipe "${URL}" | grep -oE "files/stable/q4os-${RELEASE}-${SUFFIX}[0-9]+\.iso" | sort -V | tail -1)
    echo "https://downloads.sourceforge.net/project/q4os/${ISO#files/} ${HASH}"
}

function get_systemrescue() {
    local HASH="" ISO="systemrescue-${RELEASE}-amd64.iso" URL="https://downloads.sourceforge.net/project/systemrescuecd/sysresccd-x86/${RELEASE}"
    echo "${URL}/${ISO} ${HASH}"
}

function get_truenas-scale() {
    local HASH="" ISO="" VDIR="" BASE="https://download.sys.truenas.net" DIR="TrueNAS-SCALE-${RELEASE^}"
    VDIR=$(web_pipe "${BASE}/${DIR}/" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | sort -uVr | head -1)
    ISO=$(web_pipe "${BASE}/${DIR}/${VDIR}/" | grep -oE 'TrueNAS[^"<]*\.iso' | grep -vE 'gpg|sha|mtree|wrap' | sort -u | head -1)
    HASH=$(web_pipe "${BASE}/${DIR}/${VDIR}/${ISO}.sha256" | cut -d' ' -f1)
    echo "${BASE}/${DIR}/${VDIR}/${ISO} ${HASH}"
}

function get_ponyos() {
    local ISO="image.iso"
    local URL="https://github.com/klange/ponyos/releases/download/v${RELEASE}"
    echo "${URL}/${ISO} 1ce72ca37b038e81874d090d5544ccccc453be7f188b5ee96656b754c6881385"
}


function get_nyarch() {
    local EDITION_CAP
    EDITION_CAP="$(echo "${EDITION}" | sed 's/./\u&/')"
    local ISO="Nyarch-${EDITION_CAP}-${RELEASE}.iso"
    local URL="https://sourceforge.net/projects/nyarchlinux/files"
    echo "${URL}/${ISO}/download"
}


function get_nst() {
    local ISO="nst-${RELEASE}.x86_64.iso"
    local URL="https://sourceforge.net/projects/nst/files/NST/NST%20${RELEASE}"
    echo "${URL}/${ISO}/download"
}


function get_9front() {
    local URL="https://9front.org/iso"
    local ISO
    ISO="$(web_pipe "${URL}/" | grep -o '9front-[0-9]*\\.amd64.iso' | sort -Vr | head -1)"
    echo "${URL}/${ISO}"
}


function get_securityonion() {
    local URL="https://download.securityonion.net/file/securityonion"
    local ISO
    ISO="$(web_pipe "${URL}/" | grep -o 'securityonion-[0-9.]*-[0-9]*\\.iso' | sort -Vr | head -1)"
    echo "${URL}/${ISO}"
}


function get_pentoo() {
    local URL="https://www.pentoo.ch/isos"
    local ISO
    ISO="$(web_pipe "${URL}/" | grep -o "pentoo-${EDITION}-[a-zA-Z0-9_-]*\\.iso" | sort -Vr | head -1)"
    echo "${URL}/${ISO}"
}


function get_kicksecure() {
    local BASE="https://download.kicksecure.com/kicksecure"
    local VER
    VER="$(web_pipe "${BASE}/" | grep -o 'href="17\\.[0-9.]*/' | cut -d'"' -f2 | tr -d '/' | sort -Vr | head -1)"
    local ISO
    ISO="$(web_pipe "${BASE}/${VER}/" | grep -o 'Kicksecure[^" ]*Intel_AMD64\\.iso' | head -1)"
    echo "${BASE}/${VER}/${ISO}"
}


function get_uwuntu() {
    local ISO
    ISO="$(web_pipe "https://uwuntuos.com/" | grep -o 'href="[^"]*\\.iso' | cut -d'"' -f2 | grep -i "22.10" | head -1 | xargs basename 2>/dev/null)"
    echo "https://uwuntuos.com/releases/${RELEASE}/${ISO:-uwuntu-22.10-leo-dango-amd64.iso}"
}

function get_caine() {
    local ISO="caine${RELEASE}.iso"
    local URL="https://www.caine-live.net/Downloads"
    echo "${URL}/${ISO}"
}


function get_rebeccablackos() {
    local ISO="RebeccaBlackOS_amd64.iso"
    local URL="https://sourceforge.net/projects/rebeccablackos/files/${RELEASE}"
    echo "${URL}/${ISO}/download"
}


function get_tsurugi() {
    local URL="https://ftp.nluug.nl/pub/security/DigitalForensics/tsurugi"
    local ISO
    ISO="$(web_pipe "${URL}/" | grep -o 'tsurugi_linux_[0-9.]*\\.iso' | sort -Vr | head -1)"
    echo "${URL}/${ISO:-tsurugi_linux_${RELEASE}.iso}"
}


function get_backbox() {
    local ISO
    ISO="$(web_pipe "https://linux.backbox.org/download/" | grep -o 'href="[^"]*\\.iso' | cut -d'"' -f2 | head -1)"
    echo "${ISO:-https://backbox.mirror.garr.it/backbox-${RELEASE}-desktop-amd64.iso}"
}


function get_whonix() {
    # Whonix 18+ — official libvirt bundle (contains Gateway + Workstation
    # qcow2 + XML) — extract and import both VMs from the single archive
    local BASE="https://download.whonix.org/libvirt"
    local VER="18.2.4.2"
    local ISO="Whonix-LXQt-${VER}.Intel_AMD64.qcow2.libvirt.xz"
    echo "${BASE}/${VER}/${ISO}"
}

function get_amogos() {
    local ISO="AmogOS-1.5.0-x86_64.iso"
    local URL="https://github.com/Amog-OS/AmogOS/releases/download/${RELEASE}"
    echo "${URL}/${ISO}"
}


function get_gnulinex() {
    # gnuLinEx — netinst via GitHub, classic 2003 LiveCD via archive.org
    if [ "${EDITION}" = "classic" ]; then
        echo "https://archive.org/download/GNULINEX/GNULINEX.ISO"
        return
    fi
    local ISO="gnuLinEx-amd64-netinst.iso"
    local URL="https://github.com/CarlosGamer98YT/gnuLinEx/releases/download/${RELEASE}"
    echo "${URL}/${ISO} f649fbcd8f08788ff9015ba48471b932b9328433196f8f68461c8711cf671804"
}


function get_molinux() {
    # MoLinux 6.0 Zoraida — preserved on ArchiveOS SourceForge
    local ISO="Molinux_${RELEASE}_Zoraida.iso"
    local URL="https://sourceforge.net/projects/archiveos/files/m/molinux"
    echo "${URL}/${ISO}/download"
}

function get_syllable() {
    # Syllable Desktop — SourceForge, ISO inside .iso.7z
    local ISO="SyllableDesktop-${RELEASE}-1.i586.iso.7z"
    local URL="https://sourceforge.net/projects/syllable/files/Syllable/${RELEASE}"
    echo "${URL}/${ISO}/download 6599c235ba7242bc58b2dabc69559b25c183e32629b4af2b03be7be355e45fca"
}


function get_biebian() {
    # Biebian (Justin Bieber Linux) — SourceForge, JBL_1.0.iso
    local ISO="JBL_${RELEASE}.iso"
    local URL="https://sourceforge.net/projects/biebian/files"
    echo "${URL}/${ISO}/download 80be226276a2ef02156d29ffa1a24adb561955cd8c2342666ae82164e84c590e"
}

function get_maxlinux() {
    # MAX (MAdrid_linuX) — scrape the RedIRIS MAX-<ver> dir for the ISO
    local BASE="https://ftp.rediris.es/mirror/MaX-Linux/MaXdesktop"
    local ISO
    ISO="$(web_pipe "${BASE}/MAX-${RELEASE}/" | grep -o "MAX-[^\"<> ]*\.iso" | grep -v md5 | sort -V | head -1)"
    local HASH
    HASH="$(web_pipe "${BASE}/MAX-${RELEASE}/${ISO}.md5" | cut -d' ' -f1)"
    echo "${BASE}/MAX-${RELEASE}/${ISO} ${HASH}"
}


function get_vitalinux() {
    # Vitalinux — official Aragon education mirror index + .md5 sibling
    local URL="https://mirror.vitalinux.educa.aragon.es/isos"
    local ISO
    ISO="$(web_pipe "${URL}/" | grep -o "vitalinux-${RELEASE}[^\"<> ]*desktop-amd64\.iso" | sort -Vr | head -1)"
    local HASH
    HASH="$(web_pipe "${URL}/${ISO}.md5" | cut -d' ' -f1)"
    echo "${URL}/${ISO:-vitalinux-3.5-2026.05.12-desktop-amd64.iso} ${HASH}"
}

function get_guadalinex() {
    # Guadalinex — v9 on ArchiveOS SF; older releases on archive.org
    local ISO=""
    case "${RELEASE}" in
        v8)       ISO="guadalinex-v8-desktop-i386-final.iso";;
        v7)       ISO="guadalinex-v7-desktop-i386-cd.iso";;
        v6)       ISO="guadalinex-v6-desktop-cd.iso";;
        2004)     ISO="guadalinex-2004.iso";;
        1.0)      ISO="guadalinex-1.0.iso";;
        mini2004) ISO="guadalinex-mini-2004_1.0.iso";;
        v9) ;;
    esac
    if [ -n "${ISO}" ]; then
        echo "https://archive.org/download/guadalinex-v9-32B/${ISO}"
        return
    fi
    case "${EDITION}" in
        lite) ISO="guadalinex-v9-lite-32B.iso";;
        *)    ISO="guadalinex-v9-32B.iso";;
    esac
    local URL="https://sourceforge.net/projects/archiveos/files/g/guadalinex"
    echo "${URL}/${ISO}/download"
}


function get_gecos() {
    # GECOS v2 — preserved on archive.org (Guadalinex item)
    local ISO="GECOS-v2-32B-latest.iso"
    [ "${EDITION}" = "lite" ] && ISO="GECOS-v2-lite-32B-20141017.iso"
    echo "https://archive.org/download/guadalinex-v9-32B/${ISO}"
}


function get_esfera() {
    local ISO="esfera-live-${RELEASE}.iso"
    echo "https://archive.org/download/guadalinex-v9-32B/${ISO}"
}


function get_recreomatic() {
    echo "https://archive.org/download/guadalinex-v9-32B/recreomatic.iso"
}


function get_mangaka() {
    local ISO="linux-mangaka-${RELEASE}-v2.iso"
    echo "https://archive.org/download/linux-mangaka/${ISO}"
}


function get_lliurex() {
    # LliureX — verified filenames on the official index
    local BASE="http://lliurex.net/isos/${RELEASE}"
    local ISO="LliureX-home-desktop_${RELEASE}_latest.iso"
    case "${RELEASE}" in
        23)    ISO="LliureX-${EDITION}_64bits_23_latest.iso";;
        21.07) ISO="LliureX-${EDITION}_64bits_21_20220406.iso"; BASE="http://lliurex.net/isos/21.07_64bits/20220406";;
        19.07) ISO="lliurex-${EDITION}_64bits_19_latest.iso"; BASE="http://lliurex.net/isos/19.07_64bits";;
    esac
    echo "${BASE}/${ISO}"
}


function get_hannahmontana() {
    # Hannah Montana Linux — lite via SourceForge, classic 2009 via archive.org
    if [ "${EDITION}" = "classic" ]; then
        local ISO="hannah_montana_linux_x86_basic_edition.iso"
        echo "https://archive.org/download/hannah_montana_linux_x86_basic_edition_202210/${ISO}"
        return
    fi
    local ISO="hannah-montana-linux-26-lite.iso"
    case "${RELEASE}" in
        26.1.1) ISO="hannah-montana-linux-26-1-1-lite.iso";;
    esac
    local URL="https://sourceforge.net/projects/hannah-montana-linux-v26-lite/files/v${RELEASE}"
    echo "${URL}/${ISO}/download"
}


function get_guadalinexedu() {
    # Guadalinex Edu — preserved on archive.org
    local URL="https://archive.org/download/guadalinexedu-version-archive"
    local ISO
    case "${EDITION}" in
        sdi)     ISO="guadalinexedu-2013-instalador-SDI.iso";;
        centros) ISO="guadalinexedu-2013-instalador-centros-2016012601.iso";;
        *)       ISO="guadalinexedu-${RELEASE}-desktop-amd64.iso";;
    esac
    [ "${RELEASE}" = "10.04" ] && ISO="guadalinexedu-10.04.01-desktop-amd64.iso"
    echo "${URL}/${ISO}"
}

function get_linkat() {
    # Linkat 22.04 — official xtec.cat download host
    local ISO="linkat-22.04.3-amd64.iso"
    local URL="http://download-linkat.xtec.cat/d83/ISO"
    echo "${URL}/${ISO}"
}

function get_educaandos() {
    # EducaAndOS — official cica.es mirror, verified filenames
    local BASE="http://guadalinex-edu.cica.es/centros"
    local ISO="educaandos-20.04-instalador-centros.iso"
    [ "${RELEASE}" = "v2" ] && ISO="EducaAndosV2-offline.iso"
    echo "${BASE}/${ISO}"
}

function get_minino() {
    # GALPon MiniNo — preserved on ArchiveOS SourceForge
    local ISO="minino-${RELEASE}-${EDITION}.iso"
    [ "${RELEASE}" = "artabros" ] && [ "${EDITION}" = "64" ] && ISO="minino-artabros-full.iso"
    [ "${RELEASE}" = "artabros" ] && [ "${EDITION}" = "32" ] && ISO="minino-artabros-minimal.iso"
    local URL="https://sourceforge.net/projects/archiveos/files/m/minino"
    echo "${URL}/${ISO}/download"
}

function get_subgraph() {
    # Subgraph OS alpha — preserved on ArchiveOS SourceForge
    local ISO="subgraph-os-alpha_2016-12-30_1.iso"
    local URL="https://sourceforge.net/projects/archiveos/files/s/subgraph"
    echo "${URL}/${ISO}/download"
}

function get_plan9() {
    # Plan 9 from Bell Labs — all 4 editions preserved on archive.org
    local ISO="plan9-${RELEASE}.iso"
    [ "${RELEASE}" = "4e" ] && ISO="plan9-4e-latest.iso"
    echo "https://archive.org/download/plan9isosUS/${ISO}"
}

function get_wifislax() {
    # WiFiSlax 4.10.1 — preserved on archive.org (official site uses JS hosts)
    local ISO="wifislax-4-10-1-final.iso"
    echo "https://archive.org/download/wifislax-4-10-1-final_202406/${ISO} d9010acaa7bfa2a16594f3ee0670dc85"
}

function get_os2warp() {
    # IBM OS/2 Warp 4 (1996) — preserved on archive.org; needs floppy+CD
    # boot inside QEMU (historic, no networking support expected)
    echo "https://archive.org/download/IBMOS2Warp4Collection/OS2WARP4-EN.iso"
}
