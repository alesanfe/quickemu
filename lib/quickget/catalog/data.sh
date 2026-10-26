# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: per-OS release/edition/language/architecture data functions.


function releases_alma() {
    echo 9 8
}


function editions_alma() {
    echo boot minimal dvd
}


function releases_alpine() {
    local REL=""
    local RELS=""
    RELS=$(web_pipe "https://dl-cdn.alpinelinux.org/alpine/" | grep '"v' | cut -d'"' -f2 | tr -d / | sort -Vr | head -n 10)
    for REL in ${RELS}; do
        if web_check "https://dl-cdn.alpinelinux.org/alpine/${REL}/releases/x86_64/"; then
            echo -n "${REL} "
        fi
    done
}


function releases_android() {
    echo 9.0 8.1 7.1
}


function editions_android() {
    echo x86_64 x86
}


function releases_antix() {
    echo 23.1 23 22 21
}


function editions_antix() {
    echo net-sysv core-sysv base-sysv full-sysv net-runit core-runit base-runit full-runit
}


function releases_archcraft() {
    echo latest
}


function releases_archlinux() {
    echo latest
}


function releases_artixlinux() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirror1.artixlinux.org/iso/" | grep "artix-" | cut -d'"' -f2 | grep -v sig | cut -d'-' -f 4 | sort -ru | tail -n 1)
}


function editions_artixlinux() {
    #shellcheck disable=SC2046,SC2005
    # Extract edition-init combinations (e.g., base-dinit, plasma-openrc, community-gtk-openrc)
    echo $(web_pipe "https://mirror1.artixlinux.org/iso/" | grep "artix-" | cut -d'"' -f2 | grep -v sig | sed 's/artix-//' | sed 's/-[0-9]\{8\}-x86_64.iso//' | sort -u)
}


function releases_azurelinux() {
    echo 3.0
}


function releases_batocera() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirrors.o2switch.fr/batocera/x86_64/stable/" | grep ^\<a | cut -d'"' -f2 | cut -d '/' -f1 | grep -v '\.' | sort -ru | tail -n +2 | head -n 5)
}


function releases_bazzite() {
    echo latest
}


function editions_bazzite() {
    echo gnome plasma deck-gnome deck-plasma
}


function releases_biglinux() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://iso.biglinux.com.br" | grep -Eo 'biglinux_[0-9]{4}(-[0-9]{2}){2}_k[0-9]{2,3}.iso' | cut -d'_' -f2 | sort -ru | head -n 1)
}


function editions_biglinux() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://iso.biglinux.com.br" | grep -Eo "biglinux_$(releases_biglinux)_k[0-9]{2,3}.iso" | cut -d'_' -f3 | cut -d'.' -f1 | sort -Vru)
}


function releases_blendos() {
    # there is now just a single latest iso
    echo latest
}


function releases_bodhi() {
    echo 7.0.0
}


function editions_bodhi() {
    echo standard hwe s76
}


function releases_bunsenlabs() {
    echo boron
}


function releases_cachyos() {
    # new cdn setup 10/2024
    echo latest
}


function editions_cachyos() {
    # desktop version now installs different desktop environments
    echo desktop handheld
}


function releases_centos-stream() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://linuxsoft.cern.ch/centos-stream/" | grep "\-stream" | cut -d'"' -f 6 | cut -d'-' -f 1)
}


function editions_centos-stream() {
    echo boot dvd1
}


function releases_chimeralinux() {
    echo latest
}


function editions_chimeralinux() {
    echo base gnome
}


function editions_kdeneon() {
    echo bigscreen desktop dev ko mobile
}


function releases_crunchbang++() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://api.github.com/repos/CBPP/cbpp/releases" | grep 'download_url' | cut -d'-' -f2 | grep '^[0-9]' | sort -gru)
}


function releases_debian() {
    local ARCHIVE=""
    local MAJ=""
    local NEW=""
    local OLD=""
    NEW=$(web_pipe "https://cdimage.debian.org/debian-cd/" | grep '\.[0-9]/' | cut -d'>' -f 9 | cut -d'/' -f 1)
    echo -n "${NEW}"
    MAJ=$(echo "${NEW}" | cut -d'.' -f 1)
    # Get archive versions that have -live directories (live images available)
    ARCHIVE="$(web_pipe "https://cdimage.debian.org/cdimage/archive/" | grep folder | grep -v NEVER | grep '\-live/' | cut -d'"' -f 6 | sed 's/-live\///')"
    for i in {1..2}; do
        CUR=$((MAJ - i))
        OLD=$(grep ^"${CUR}"  <<< "${ARCHIVE}" | tail -n 1 | tr -d '/')
        echo -n " ${OLD}"
    done
    echo
}


function editions_debian() {
    echo standard cinnamon gnome kde lxde lxqt mate xfce netinst
}


function releases_deepin() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirrors.kernel.org/deepin-cd/" | grep "href=" | cut -d'"' -f2 | grep -v "\.\." | grep -v nightly | grep -v preview | sed 's|/||g' | tail -n 10 | sort -r)
}


function releases_devuan() {
    echo daedalus chimaera
}


function releases_dragonflybsd() {
    # If you remove "".bz2" from the end of the searched URL, you will get only the current release - currently 6.4.0
    # We could add a variable so this behaviour is optional/switchable (maybe from option or env)
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirror-master.dragonflybsd.org/iso-images/" | grep -E -o '"dfly-x86_64-.*_REL.iso.bz2"' | grep -o -E '[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+')
}


function releases_dragonos() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/dragonos-focal/rss?path=/" | grep -oE 'DragonOS_[A-Za-z]+_R[0-9]+' | sort -uVr | sed 's/DragonOS_//')
}


function releases_easyos() {
    local ALL_RELEASES=""
    local YEAR=""
    # get the latest 2 years of releases so that when we hit next year we still have the latest 2 years
    TWO_YEARS=$(web_pipe https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/ | grep -o -E '[[:digit:]]{4}/' | sort -nr | tr -d /  | head -n 2 )
    for YEAR in ${TWO_YEARS} ; do
        ALL_RELEASES="${ALL_RELEASES} $(web_pipe "https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/${YEAR}/" | grep -o -E '[[:digit:]]+(\.[[:digit:]])+/' | tr -d / | sort -nr)"
    done
    echo "${ALL_RELEASES}"
}



function releases_elementary() {
    echo 8.1 8.0 7.1 7.0
}


function releases_endeavouros() {
    local ENDEAVOUR_RELEASES=""
    ENDEAVOUR_RELEASES="$(web_pipe "https://mirror.alpix.eu/endeavouros/iso/" | grep -o '<a href="[^"]*.iso">' | sed 's/^<a href="//;s/.iso">.*//' | grep -v 'x86_64' | LC_ALL="en_US.UTF-8" sort -Mr | cut -c 13- | head -n 5 | tr '\n' ' ')"
    echo "${ENDEAVOUR_RELEASES,,}"
}


function releases_endless() {
    echo 6.0.4
}


function editions_endless() {
    echo base en fr pt_BR es
}


function releases_fedora() {
    #shellcheck disable=SC2046,SC2005
    # Filter out EOL release 41
    echo $(web_pipe "https://getfedora.org/releases.json" | jq -r 'map(.version) | unique | .[]' | sed 's/ /_/g' | sort -r | grep -v '^41$')
}


function editions_fedora() {
    #shellcheck disable=SC2046,SC2005
    if [[ -z ${RELEASE} ]]; then
          echo $(web_pipe "https://getfedora.org/releases.json" | jq -r "map(select(.arch==\"x86_64\" and .variant!=\"Labs\" and .variant!=\"IoT\" and .variant!=\"Container\" and .variant!=\"Cloud\" and .variant!=\"Everything\"  and .subvariant!=\"Security\" and .subvariant!=\"Server_KVM\" and .subvariant!=\"SoaS\")) | map(.subvariant) | unique | .[]")
    else
        echo $(web_pipe "https://getfedora.org/releases.json" | jq -r "map(select(.arch==\"x86_64\" and .version==\"${RELEASE/_/ }\" and .variant!=\"Labs\" and .variant!=\"IoT\" and .variant!=\"Container\" and .variant!=\"Cloud\" and .variant!=\"Everything\"  and .subvariant!=\"Security\" and .subvariant!=\"Server_KVM\" and .subvariant!=\"SoaS\")) | map(.subvariant) | unique | .[]")
    fi
}


function releases_freebsd() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://download.freebsd.org/ftp/releases/amd64/amd64/" | grep -Eo "href=\"[0-9\.]+-RELEASE" | grep -oE '[0-9\.]+' | sort -r)
}


function editions_freebsd() {
    echo disc1 dvd1
}


function releases_freedos() {
    echo 1.4 1.3 1.2
}


function releases_garuda() {
    echo latest
}


function editions_garuda() {
    echo cinnamon dr460nized dr460nized-gaming gnome hyprland i3 kde-lite mokka sway xfce
}


function releases_gentoo() {
    echo latest
}


function editions_gentoo() {
    echo minimal livegui
}


function releases_ghostbsd() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://download.ghostbsd.org/releases/amd64/" | grep "href" | cut -d'"' -f2 | cut -d'/' -f1 | sort -r | tail -n +3 | head -n 3)
}


function editions_ghostbsd() {
    echo mate xfce
}


function releases_gobolinux() {
    echo 017.01
}


function releases_gnomeos() {
    #shellcheck disable=SC2046,SC2005
    # Filter out alpha/beta/rc pre-releases, keep only stable versions and nightly
    echo "nightly" $(web_pipe "https://download.gnome.org/gnomeos/" | grep "title=" | awk -F'"' '{print $4}' | tr -d '/' | grep -E '^[0-9]+\.[0-9]+$' | sort -nr)
}


function releases_guix() {
    echo 1.5.0 1.4.0
}


function releases_haiku() {
    echo r1beta5 r1beta4 r1beta3
}


function editions_haiku() {
    echo x86_64 x86_gcc2h
}


function releases_kali() {
    echo current kali-weekly
}


function editions_kali() {
    echo standard purple netinst
}


function releases_kdeneon() {
    echo user testing unstable
}


function releases_kdelinux() {
    echo latest
}


function releases_kolibrios() {
    echo latest
}


function releases_templeos() {
    echo 5.03
}


function editions_kolibrios() {
    echo en_US ru_RU es_ES
}


function releases_linuxlite() {
    echo 6.6 6.4 6.2 6.0
}


function releases_linuxmint() {
    echo 22.1 22 21.3 21.2 21.1 21 20.3 20.2
}


function editions_linuxmint() {
    echo cinnamon mate xfce
}


function editions_lmde() {
    echo cinnamon
}


function releases_lmde() {
    echo 6
}


function releases_maboxlinux() {
    echo latest
}


function releases_macos() {
    echo mojave catalina big-sur monterey ventura sonoma sequoia tahoe
}


function releases_mageia() {
    echo 9
}


function editions_mageia() {
    echo Plasma GNOME Xfce
}


function editions_manjaro() {
    echo full minimal
}


function releases_manjaro() {
    echo xfce gnome plasma cinnamon i3 sway
}


function releases_mxlinux() {
    # needs header, so not web_pipe:
    curl --disable -Ils "https://sourceforge.net/projects/mx-linux/files/latest/download" | grep -i 'location:' | cut -d? -f1 | cut -d_ -f1 | cut -d- -f3
}


function editions_mxlinux() {
    echo Xfce KDE Fluxbox
}


function releases_netboot() {
    echo latest
}


function releases_netbsd() {
    # V8 is EOL so filter it out
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://cdn.netbsd.org/pub/NetBSD/iso/" | grep -o -E '"[[:digit:]]+\.[[:digit:]]+/"' | tr -d '"/' | grep -v ^8 | sort -nr | head -n 4)
}


function releases_nitrux() {
    echo latest
}


function releases_nixos() {
    # Lists unstable plus the most recent release
    #shellcheck disable=SC2046
    echo unstable $(web_pipe "https://nix-channels.s3.amazonaws.com/?delimiter=/" | grep -o -E 'nixos-[[:digit:]]+\.[[:digit:]]+' | cut -d- -f2 | sort -nru | head -n +1)
}


function editions_nixos() {
    echo minimal graphical
}


function releases_nwg-shell() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/nwg-iso/rss?path=/" | grep 'url=' | grep '64.iso' | cut -d'/' -f12 | cut -d'-' -f3)
}


function releases_omarchy() {
    echo 4.0.0
}


function releases_openfyde() {
    web_pipe "https://api.github.com/repos/openfyde/overlay-amd64-openfyde/releases" | jq -r '.[].tag_name' | tr '\n' ' '
}


function releases_openbsd() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirror.leaseweb.com/pub/OpenBSD/" | grep -e '6\.[8-9]/' -e '[7-9]\.' | cut -d\" -f4 | tr -d '/' | sort -r)
}


function releases_openindiana() {
    #shellcheck disable=SC2046,SC2005
    #echo $(web_pipe "https://dlc.openindiana.org/isos/hipster/" | grep link | cut -d'/' -f 1 | cut -d '"' -f4 | sort -r | tail -n +2 | head -n 5)
    echo $(web_pipe "https://dlc.openindiana.org/isos/hipster/" | grep 'href="./2' | cut -d'/' -f 2 | sort -r |  head -n 5)

}


function editions_openindiana() {
    echo gui text minimal
}


function releases_opensuse() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://download.opensuse.org/distribution/leap/" | grep 'class="name"' | cut -d '/' -f2 | grep -v 42 | sort -r) microos tumbleweed
}


function releases_oraclelinux() {
    echo 9.3 9.2 9.1 9.0 8.9 8.8 8.7 8.6 8.5 8.4 7.9 7.8 7.7
}


function releases_parrotsec() {
    #shellcheck disable=SC2046,SC2005
    # Only return releases that have ISO files available
    local RELEASES=""
    for REL in $(web_pipe "https://deb.parrot.sh/parrot/iso/" | grep -o -E 'href="([[:digit:]]+\.)+[[:digit:]]+/' | sort -Vr | head -n 5 | cut -d\" -f 2 | cut -d'/' -f 1); do
        if web_pipe "https://download.parrot.sh/parrot/iso/${REL}/" | grep -q '\.iso"'; then
            RELEASES="${RELEASES} ${REL}"
        fi
    done
    echo "${RELEASES}"
}


function editions_parrotsec() {
    # htb edition was discontinued after 6.x series
    echo home security
}


function releases_pclinuxos() {
    # Use 'latest' as different editions have different release dates
    echo latest
}


function editions_pclinuxos() {
    echo kde kde-darkstar mate xfce
}


function releases_peppermint() {
    echo latest
}


function editions_peppermint() {
    echo devuan-xfce devuan-gnome debian-xfce debian-gnome
}


function releases_popos() {
    echo 22.04 20.04 24.04
}


function editions_popos() {
    echo intel nvidia
}


function releases_porteus() {
    echo 5.01
}


function editions_porteus() {
    echo cinnamon gnome kde lxde lxqt mate openbox xfce
}


function releases_primtux() {
    echo 7
}


function editions_primtux() {
    echo 2022-10
}


function releases_proxmox-ve() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe https://enterprise.proxmox.com/iso/ | grep proxmox-ve | grep -E -o '[0-9]+\.[0-9]+-[0-9]\.iso' | uniq | sort -ru | cut -d'.' -f 1-2)
}


function releases_pureos() {
    web_pipe "https://www.pureos.net/download/" | grep -m 1 "downloads.puri" | cut -d '"' -f 2 | cut -d '-' -f 4
}


function editions_pureos() {
    echo gnome plasma
}


function releases_qubes() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://ftp.qubes-os.org/iso/" | grep -oE 'R4\.[0-9.]+-x86_64\.iso' | grep -oE '4\.[0-9.]+' | sort -uVr)
}


function releases_redoxos() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://static.redox-os.org/releases/" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | sort -uVr)
}


function editions_redoxos() {
    echo "desktop server demo"
}


function releases_reactos() {
    echo latest
}


function releases_rebornos() {
    echo latest
}


function releases_rockylinux() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "http://dl.rockylinux.org/vault/rocky/" | grep "class=\"link" | grep -v -e 'full' -e  'RC' -e  'ISO' -e 'Parent' | cut -d'"' -f4 | tr -d / | sort -ru)
}


function editions_rockylinux() {
    echo minimal dvd boot
}


function releases_siduction() {
    echo latest
}


function editions_siduction() {
    #shellcheck disable=SC2046,SC2005
    NAME=$(web_pipe "https://mirror.math.princeton.edu/pub/siduction/iso/" | grep folder | cut -d'"' -f8 | tr -d '/')
    web_pipe "https://mirror.math.princeton.edu/pub/siduction/iso/${NAME}/" | grep folder | cut -d'"' -f8 | tr -d '/' | sort -u
}


function releases_slackware() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://slackware.nl/slackware/slackware-iso/" | grep "slackware-" | cut -d'<' -f7 | cut -d'-' -f2 | sort -ru | head -n 5)
}


function releases_slax() {
    echo latest
}


function editions_slax() {
    echo debian slackware
}


function releases_slint() {
    echo "15.0-10"
}


function releases_slitaz() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirror.slitaz.org/iso/rolling/" | grep "class='iso'" | cut -d"'" -f4 | cut -d'-' -f3- | grep iso | cut -d'.' -f1 | sort -u)
}


function releases_solus() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe_json "https://downloads.getsol.us/isos/" | jq -r '.[].name[:-1]' | grep -E "^[0-9-]+$" | sort -u)
}


function editions_solus() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe_json "https://downloads.getsol.us/isos/$(IFS=' '; releases_solus | tail -n1)/" | jq -r '.[].name | select(endswith("iso")) | sub("Solus-(?<e>.*)-Release-.*"; "\(.e)")' | sort -u)
}


function releases_sparkylinux() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sparkylinux.org/download/stable/" |  grep -E -o "sparkylinux-.*\.iso\"" | cut -d'-' -f2 | sort -ru)
}


function editions_sparkylinux() {
    #shellcheck disable=SC2046,SC2005
    if [ -z "${RELEASE}" ]; then
        echo $(web_pipe "https://sparkylinux.org/download/stable/" | grep -E -o "sparkylinux-.*\.iso\"" | cut -d'-' -f4 | cut -d'.' -f1 | sort -u)
    else
        echo $(web_pipe "https://sparkylinux.org/download/stable/" | grep -E -o "sparkylinux-${RELEASE}-.*\.iso\"" | cut -d'-' -f4 | cut -d'.' -f1 | sort -u)
    fi

}


function releases_spirallinux() {
    echo latest
}


function editions_spirallinux() {
    echo Plasma XFCE Mate LXQt Gnome Budgie Cinnamon Builder
}


function releases_tails() {
    echo stable
}


function releases_tinycore() {
    echo 15 14
}


function editions_tinycore() {
    echo Core TinyCore CorePlus CorePure64 TinyCorePure64
}


function releases_trisquel() {
    echo 11.0 10.0.1
}


function editions_trisquel() {
    echo mate lxde kde sugar
}


function releases_tuxedo-os() {
    echo current
}


function releases_ubuntu() {
    local VERSION_DATA=""
    local SUPPORTED_VERSIONS=()
    VERSION_DATA="$(IFS=$'\n' web_pipe https://api.launchpad.net/devel/ubuntu/series | jq -r '.entries[]')"
    # shellcheck disable=SC2207
    SUPPORTED_VERSIONS=($(IFS=$'\n' jq -r 'select(.status=="Supported" or .status=="Current Stable Release") | .version' <<<"${VERSION_DATA}" | sort))
    case "${OS}" in
        ubuntu)
            echo "${SUPPORTED_VERSIONS[@]}" daily-live;;
        kubuntu|lubuntu|ubuntukylin|ubuntu-mate|ubuntustudio|xubuntu)
            # after 16.04
            echo "${SUPPORTED_VERSIONS[@]:1}" daily-live;;
        ubuntu-budgie)
            # after 18.04
            echo "${SUPPORTED_VERSIONS[@]:2}" daily-live;;
        edubuntu|ubuntu-unity|ubuntucinnamon)
            # after 23.10
            echo "${SUPPORTED_VERSIONS[@]:5}" daily-live;;
    esac
}


function releases_ubuntu-server() {
    local ALL_VERSIONS=()
    # shellcheck disable=SC2207
    ALL_VERSIONS=($(IFS=$'\n' web_pipe http://releases.ubuntu.com/streams/v1/com.ubuntu.releases:ubuntu-server.json | jq -r '.products[] | select(.arch=="amd64") | .version' | sort -rV))
    echo daily-live "${ALL_VERSIONS[@]}"
}


function releases_vanillaos() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://api.github.com/repos/Vanilla-OS/live-iso/releases" | grep 'download_url' | cut -d'/' -f8 | sort -ru)
}


function releases_void() {
    # List directories that contain standard ISO files (base.iso), not just enterprise editions
    #shellcheck disable=SC2046,SC2005
    local DIRS=""
    DIRS=$(web_pipe "https://repo-default.voidlinux.org/live/" | grep "^<a href=\"2" | cut -d'"' -f2 | tr -d '/' | sort -ru | head -n 5)
    for DIR in ${DIRS}; do
        # Only include directories that have standard base ISOs
        if web_pipe "https://repo-default.voidlinux.org/live/${DIR}/" | grep -q "base.iso"; then
            echo -n "${DIR} "
        fi
    done
}


function editions_void() {
    echo glibc musl xfce-glibc xfce-musl
}


function releases_windows() {
    echo 11 10
}


function languages_windows() {
    I18NS=("Arabic" "Brazilian Portuguese" "Bulgarian" "Chinese (Simplified)" "Chinese (Traditional)" "Croatian" "Czech" "Danish" "Dutch" \
    "English (United States)" "English International" "Estonian" "Finnish" "French" "French Canadian" "German" "Greek" "Hebrew" "Hungarian" \
    "Italian" "Japanese" "Korean" "Latvian" "Lithuanian" "Norwegian" "Polish" "Portuguese" "Romanian" "Russian" "Serbian Latin" "Slovak" \
    "Slovenian" "Spanish" "Spanish (Mexico)" "Swedish" "Thai" "Turkish" "Ukrainian")
}


function releases_windows-server() {
    echo 2022 2019 2016
}


function languages_windows-server() {
    I18NS=("English (United States)" "Chinese (Simplified)" "French" "German" "Italian" "Japanese" "Russian" "Spanish")
}


function releases_zorin() {
    echo 18 17 16
}


# Architecture support functions
# These declare which architectures each distro supports.
# Distros without an arch_*() function default to "amd64" only.

function arch_alma() {
    echo "amd64 arm64"
}


function arch_alpine() {
    echo "amd64 arm64"
}


function arch_azurelinux() {
    echo "amd64 arm64"
}


function arch_debian() {
    case "${EDITION}" in
        netinst) echo "amd64 arm64";;
        *) echo "amd64";;
    esac
}


function arch_fedora() {
    case "${EDITION}" in
        Onyx) echo "amd64";;
        *) echo "amd64 arm64";;
    esac
}


function arch_ubuntu-server() {
    echo "amd64 arm64"
}


function arch_ubuntu() {
    # Ubuntu Desktop ARM64 available for 24.04 LTS and 25.10 onwards
    local MAJOR MINOR
    # When RELEASE is unset (initial arch check), return all supported architectures
    # Actual validation happens in get_ubuntu() when release is known
    if [ -z "${RELEASE}" ]; then
        echo "amd64 arm64"
        return
    fi
    # Non-numeric releases (daily-live, etc.) default to amd64 only
    if [[ ! "${RELEASE}" =~ ^[0-9]+\.[0-9]+$ ]]; then
        echo "amd64"
        return
    fi
    MAJOR="${RELEASE%%.*}"
    MINOR="${RELEASE##*.}"
    # 24.04 LTS and 25.10 onwards support ARM64
    if [ "${RELEASE}" == "24.04" ] || [ "${MAJOR}" -gt 25 ] || { [ "${MAJOR}" -eq 25 ] && [ "${MINOR}" -ge 10 ]; }; then
        echo "amd64 arm64"
    else
        echo "amd64"
    fi
}


function editions_zorin() {
    # Lite edition not available for Zorin 18 (Pro-only starting from 18)
    # When RELEASE is unset (e.g. csv_data context), return all editions
    if [ -z "${RELEASE}" ] || [ "${RELEASE}" != "18" ]; then
        echo core64 lite64 education64
    else
        echo core64 education64
    fi
}


# Returns the newest Ubuntu codename that hosts daily-live images for
# the given OS. Ubuntu moved daily images into per-codename directories
# (e.g. cdimage.ubuntu.com/ubuntu/stonking/daily-live/current), so the
# old flat "daily-live" directory no longer exists.
function ubuntu_daily_codename() {
    local OS="$1"
    local NAME=""
    local NAMES=()
    # Series are listed newest first
    # shellcheck disable=SC2207
    NAMES=($(web_pipe https://api.launchpad.net/devel/ubuntu/series | jq -r '.entries[].name'))
    for NAME in "${NAMES[@]}"; do
        if web_check "https://cdimage.ubuntu.com/${OS}/${NAME}/daily-live/current/SHA256SUMS"; then
            echo "${NAME}"
            return
        fi
    done
}


function releases_nobara() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://nobaraproject.org/download-nobara/" | grep -oE 'Nobara-[0-9]+-' | sort -uVr | sed 's/Nobara-//;s/-$//')
}

function releases_opnsense() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://mirror.fra10.de.leaseweb.net/opnsense/releases/" | grep -oE 'href="[0-9.]+/"' | tr -dc '0-9.\n' | sort -uVr | head -5)
}

function releases_vyos() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://api.github.com/repos/vyos/vyos-nightly-build/releases?per_page=5" | grep -oE '"tag_name": *"[^"]*"' | cut -d'"' -f4)
}

function releases_blackarch() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://ftp.halifax.rwth-aachen.de/blackarch/iso/" | grep -oE 'blackarch-linux-[a-z]+-[0-9.]+-x86_64' | sed 's/blackarch-linux-[a-z]*-//;s/-x86_64//' | sort -uVr)
}

function editions_blackarch() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://ftp.halifax.rwth-aachen.de/blackarch/iso/" | grep -oE "blackarch-linux-[a-z]+-${RELEASE}-x86_64" | sed 's/blackarch-linux-//;s/-x86_64//;s/-[0-9.]*$//')
}

function releases_blissos() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/blissos-x86/rss?path=/" | grep -oE 'Bliss-v[0-9]+(\.[0-9]+)+' | sort -uVr | sed 's/Bliss-v//')
}

function releases_kaos() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/kaosx/rss?path=/ISO" | grep -oE 'KaOS-[0-9.]+-x86_64\.iso' | sort -uVr | sed 's/KaOS-//;s/-x86_64\.iso//')
}

function releases_midnightbsd() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://discovery.midnightbsd.org/ftp/releases/amd64/ISO-IMAGES/" | grep -oE 'href="[0-9.]+/"' | tr -dc '0-9.\n' | sort -uVr | grep -E '^(3|4)\.' | head -5)
}

function releases_openmediavault() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/openmediavault/rss?path=/iso" | grep -oE 'openmediavault_[0-9.]+-amd64' | sort -uVr | sed 's/openmediavault_//;s/-amd64//')
}

function releases_q4os() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/q4os/rss?path=/stable" | grep -oE 'q4os-[0-9]+\.[0-9]+-x64' | sort -uVr | sed 's/q4os-//;s/-x64//')
}

function releases_systemrescue() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://sourceforge.net/projects/systemrescuecd/rss?path=/sysresccd-x86" | grep -oE 'systemrescue-[0-9.]+-amd64' | sort -uVr | sed 's/systemrescue-//;s/-amd64//')
}

function releases_truenas-scale() {
    #shellcheck disable=SC2046,SC2005
    echo $(web_pipe "https://download.sys.truenas.net/" | grep -oE 'TrueNAS-SCALE-[A-Z][a-z]+/' | sed 's/TrueNAS-SCALE-//;s|/||' | tr '[:upper:]' '[:lower:]' | sort -uVr)
}
function editions_nobara() { echo Official GNOME KDE Steam-HTPC Steam-Handheld; }

function releases_openmandriva() { echo rome; }

function editions_openmandriva() { echo plasma plasma-wayland gnome lxqt; }

function editions_q4os() { echo kde trinity; }

function releases_ponyos() {
    echo 9.0
}


function releases_nyarch() {
    echo 26.04
}


function editions_nyarch() {
    echo gnome kde
}


function releases_nst() {
    echo 44-15105
}


function releases_9front() {
    echo latest
}


function releases_securityonion() {
    echo latest
}


function releases_pentoo() {
    echo latest
}


function editions_pentoo() {
    echo full core
}


function releases_kicksecure() {
    echo latest
}


function releases_uwuntu() {
    echo 22.10
}

function releases_caine() {
    echo 14.0
}


function releases_rebeccablackos() {
    echo 2026-02-22
}


function releases_tsurugi() {
    echo 26.03
}


function releases_backbox() {
    echo 9
}


function releases_whonix() {
    echo latest
}



function releases_amogos() {
    echo x64-1.5.0
}


function releases_gnulinex() {
    echo v0.0.1
}

function releases_syllable() {
    echo 0.6.7
}


function releases_biebian() {
    echo 1.0
}

function releases_maxlinux() {
    echo 12.5 12 11.5.2
}


function releases_vitalinux() {
    echo 3.5 3.4 3.3 3.2 2.2
}

function releases_guadalinex() {
    echo v9 v8 v7 v6 2004 1.0 mini2004
}


function editions_guadalinex() {
    # v9 splits standard/lite; older releases are a single image
    # exposed under the benign "standard" edition name
    [ "${RELEASE}" = "v9" ] && echo standard lite || echo standard
}


function releases_lliurex() {
    echo 25 23 21.07
}


function editions_lliurex() {
    # 25: single home-desktop image. 23: desktop/server/client.
    # 21.07: desktop/client/server/infantil/multimedia (+fp variants)
    case "${RELEASE}" in
        23)    echo desktop server client;;
        21.07) echo desktop client server infantil multimedia;;
        *)     echo desktop;;
    esac
}


function releases_hannahmontana() {
    echo 26.1.1
}

function releases_linkat() {
    echo 22.04
}

function releases_guadalinexedu() {
    echo 2013 10.04 9.04
}


function editions_guadalinexedu() {
    echo desktop sdi centros
}


function editions_hannahmontana() {
    echo lite classic
}

function releases_molinux() {
    echo 6.0
}


function editions_molinux() {
    echo zoraida
}


function editions_gnulinex() {
    echo netinst classic
}

function releases_educaandos() {
    echo 20.04 v2
}

function releases_minino() {
    echo queiles artabros
}


function editions_minino() {
    echo 64 32 minimal full
}

function releases_subgraph() {
    echo alpha
}

function releases_plan9() {
    echo 4e 3e 2e 1e
}

function releases_gecos() {
    echo v2
}


function editions_gecos() {
    echo standard lite
}


function releases_esfera() {
    echo 1.1.0
}


function releases_recreomatic() {
    echo latest
}


function releases_mangaka() {
    echo chu
}

function releases_wifislax() {
    echo 4.10.1
}

function releases_os2warp() {
    echo 4
}

