# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: OS metadata and descriptions.


function os_info() {
    local SIMPLE_NAME=""
    local INFO=""
    SIMPLE_NAME="${1}"
    case ${SIMPLE_NAME} in
        #name)            INFO="PrettyName|Credentials|Homepage|Info";;
        altlinux)         INFO="ALT Linux|-|https://www.altlinux.org/|Veteran Russian national Linux distribution.";;
        alma)             INFO="AlmaLinux|-|https://almalinux.org/|Community owned and governed, forever-free enterprise Linux distribution, focused on long-term stability, providing a robust production-grade platform. AlmaLinux OS is binary compatible with RHEL®.";;
        alpine)           INFO="Alpine Linux|-|https://alpinelinux.org/|Security-oriented, lightweight Linux distribution based on musl libc and busybox.";;
        android)          INFO="Android x86|-|https://www.android-x86.org/|Port Android Open Source Project to x86 platform.";;
        antix)            INFO="Antix|-|https://antixlinux.com/|Fast, lightweight and easy to install systemd-free linux live CD distribution based on Debian Stable for Intel-AMD x86 compatible systems.";;
        archcraft)        INFO="Archcraft|-|https://archcraft.io/|Yet another minimal Linux distribution, based on Arch Linux.";;
        archlinux)        INFO="Arch Linux|-|https://archlinux.org/|Lightweight and flexible Linux® distribution that tries to Keep It Simple.";;
        artixlinux)       INFO="Artix Linux|-|https://artixlinux.org/|The Art of Linux. Simple. Fast. Systemd-free.";;
        azurelinux)       INFO="Azure Linux|-|https://github.com/microsoft/azurelinux|Microsoft's internal Linux distribution for cloud infrastructure and edge.";;
        batocera)         INFO="Batocera|-|https://batocera.org/|Retro-gaming distribution with the aim of turning any computer/nano computer into a gaming console during a game or permanently.";;
        bazzite)          INFO="Bazzite|-|https://github.com/ublue-os/bazzite/|Container native gaming and a ready-to-game SteamOS like.";;
        biglinux)         INFO="BigLinux|-|https://www.biglinux.com.br/|Is the right choice if you want to have an easy and enriching experience with Linux. It has been perfected over more than 19 years, following our motto: 'In search of the perfect system'.";;
        blackarch)        INFO="BlackArch|-|https://blackarch.org/|Arch-based penetration testing distribution for security researchers.";;
        blendos)          INFO="BlendOS|-|https://blendos.co/|A seamless blend of all Linux distributions. Allows you to have an immutable, atomic and declarative Arch Linux system, with application support from several Linux distributions & Android.";;
        blissos)          INFO="Bliss OS|-|https://blissos.org/|Android-based operating system for PCs and tablets.";;
        bodhi)            INFO="Bodhi|-|https://www.bodhilinux.com/|Lightweight distribution featuring the fast & fully customizable Moksha Desktop.";;
        bunsenlabs)       INFO="BunsenLabs|-|https://www.bunsenlabs.org/|Light-weight and easily customizable Openbox desktop. The project is a community continuation of CrunchBang Linux.";;
        cachyos)          INFO="CachyOS|-|https://cachyos.org/|Designed to deliver lightning-fast speeds and stability, ensuring a smooth and enjoyable computing experience every time you use it.";;
        centos-stream)    INFO="CentOS Stream|-|https://www.centos.org/centos-stream/|Continuously delivered distro that tracks just ahead of Red Hat Enterprise Linux (RHEL) development, positioned as a midstream between Fedora Linux and RHEL.";;
        chimeralinux)     INFO="Chimera Linux|anon:chimera root:chimera|https://chimera-linux.org/|Modern, general-purpose non-GNU Linux distribution.";;
        crunchbang++)     INFO="Crunchbangplusplus|-|https://www.crunchbangplusplus.org/|The classic minimal crunchbang feel, now with debian 12 bookworm.";;
        debian)           INFO="Debian|-|https://www.debian.org/|Complete Free Operating System with perfect level of ease of use and stability.";;
        deepin)           INFO="Deepin|-|https://www.deepin.org/|Beautiful UI design, intimate human-computer interaction, and friendly community environment make you feel at home.";;
        devuan)           INFO="Devuan|-|https://www.devuan.org/|Fork of Debian without systemd that allows users to reclaim control over their system by avoiding unnecessary entanglements and ensuring Init Freedom.";;
        dragonos)         INFO="DragonOS|-|https://dragonos.io/|Debian-based Linux distribution focused on software defined radio (SDR).";;
        dragonflybsd)     INFO="DragonFlyBSD|-|https://www.dragonflybsd.org/|Provides an opportunity for the BSD base to grow in an entirely different direction from the one taken in the FreeBSD, NetBSD, and OpenBSD series.";;
        easyos)           INFO="EasyOS|-|https://easyos.org/|Experimental distribution designed from scratch to support containers.";;
        edubuntu)         INFO="Edubuntu|-|https://www.edubuntu.org/|Stable, secure and privacy conscious option for schools.";;
        elementary)       INFO="elementary OS|-|https://elementary.io/|Thoughtful, capable, and ethical replacement for Windows and macOS.";;
        endeavouros)      INFO="EndeavourOS|-|https://endeavouros.com/|Provides an Arch experience without the hassle of installing it manually for both x86_64 and ARM systems.";;
        endless)          INFO="Endless OS|-|https://www.endlessos.org/os|Completely Free, User-Friendly Operating System Packed with Educational Tools, Games, and More.";;
        fedora)           INFO="Fedora|-|https://www.fedoraproject.org/|Innovative platform for hardware, clouds, and containers, built with love by you.";;
        fedoracore)       INFO="Fedora Core|-|https://archives.fedoraproject.org/|Historic Fedora Core 1-6 (pre-Fedora 7), official archive.";;
        freebsd)          INFO="FreeBSD|-|https://www.freebsd.org/|Operating system used to power modern servers, desktops, and embedded platforms.";;
        freedos)          INFO="FreeDOS|-|https://freedos.org/|DOS-compatible operating system that you can use to play classic DOS games, run legacy business software, or develop embedded systems.";;
        garuda)           INFO="Garuda Linux|-|https://garudalinux.org/|Feature rich and easy to use Linux distribution.";;
        gentoo)           INFO="Gentoo|-|https://www.gentoo.org/|Highly flexible, source-based Linux distribution.";;
        ghostbsd)         INFO="GhostBSD|-|https://www.ghostbsd.org/|Simple, elegant desktop BSD Operating System.";;
        gnomeos)          INFO="GNOME OS|-|https://os.gnome.org/|Alpha nightly bleeding edge distro of GNOME";;
        gobolinux)        INFO="GoboLinux|-|https://gobolinux.org/|Alternative Linux distribution which redefines the filesystem hierarchy.";;
        guix)             INFO="Guix|-|https://guix.gnu.org/|Distribution of the GNU operating system developed by the GNU Project—which respects the freedom of computer users.";;
        haiku)            INFO="Haiku|-|https://www.haiku-os.org/|Specifically targets personal computing. Inspired by the BeOS, Haiku is fast, simple to use, easy to learn and yet very powerful.";;
        kali)             INFO="Kali|-|https://www.kali.org/|The most advanced Penetration Testing Distribution.";;
        kaos)             INFO="KaOS|-|https://kaosx.us/|Independent rolling Linux distribution focused on KDE and Qt.";;
        kdeneon)          INFO="KDE Neon|-|https://neon.kde.org/|Latest and greatest of KDE community software packaged on a rock-solid base.";;
        kdelinux)         INFO="KDE Linux|-|https://kde.org/linux/|A free Linux-based operating system built by KDE.";;
        kolibrios)        INFO="KolibriOS|-|https://kolibrios.org/en/|Tiny yet incredibly powerful and fast operating system.";;
        guadalinex)       INFO="Guadalinex|-|https://www.guadalinex.org/|Historic education distro of Andalusia (preserved on ArchiveOS).";;
        guadalinexedu)    INFO="Guadalinex Edu|-|https://archive.org/details/guadalinexedu-version-archive|Guadalinex education spin, preserved on archive.org.";;
        educaandos)       INFO="EducaAndOS|-|https://www.juntadeandalucia.es/educacion/|Successor of Guadalinex Edu by the Andalusian government.";;
        esfera)           INFO="Esfera|-|https://archive.org/details/guadalinex-v9-32B|Historic Guadalinex-derived live distro (archive.org).";;
        gecos)            INFO="GECOS|-|https://archive.org/details/guadalinex-v9-32B|GECOS v2, Andalusian distro spin (archive.org).";;
        mangaka)          INFO="Linux Mangaka|-|https://archive.org/details/linux-mangaka|Ubuntu-based anime-themed distro (archive.org).";;
        recreomatic)      INFO="Recreomatic|-|https://archive.org/details/guadalinex-v9-32B|Guadalinex-related live distro (archive.org).";;
        hannahmontana)    INFO="Hannah Montana Linux|-|https://sourceforge.net/projects/hannah-montana-linux-v26-lite/|Debian-based meme distro, 2026 revival.";;
        lliurex)          INFO="LliureX|-|https://lliurex.net/|Education distro of the Valencian regional government.";;
        linkat)           INFO="Linkat|-|https://projectes.xtec.cat/linkat/|Education distro of the Catalan Department of Education.";;
        maxlinux)         INFO="MAX|-|https://www.educa2.madrid.org/web/max|MAdrid_linuX — education distro by the Madrid regional government.";;
        molinux)          INFO="MoLinux|-|https://archiveos.org/molinux/|Historic Castilla-La Mancha education distro (preserved on ArchiveOS).";;
        minino)           INFO="GALPon MiniNo|-|https://minino.galpon.org/|Lightweight Galician distro for old PCs (preserved on ArchiveOS).";;
        vitalinux)        INFO="Vitalinux|-|https://vitalinux.educa.aragon.es/|Education distribution by the Aragon government.";;
        biebian)          INFO="Biebian|-|https://biebian.sourceforge.net/|Justin Bieber Linux, 2011 meme distro.";;
        syllable)         INFO="Syllable|-|https://syllable.org/|Fast, lightweight non-Linux desktop OS (AtheOS descendant).";;
        subgraph)         INFO="Subgraph OS|-|https://archiveos.org/subgraph/|Hardened Debian-based privacy OS (historic 2016 alpha).";;
        amogos)           INFO="AmogOS|-|https://github.com/Amog-OS/AmogOS|Meme Linux distribution based on Among Us theming.";;
        gnulinex)         INFO="gnuLinEx|-|https://github.com/CarlosGamer98YT/gnuLinEx|Community continuation of the Extremadura gnuLinEx distribution.";;
        backbox)          INFO="BackBox|-|https://linux.backbox.org/|Ubuntu-based pentesting and security assessment distribution.";;
        caine)            INFO="CAINE|-|https://www.caine-live.net/|Digital forensics live distribution (Computer Aided INvestigative Environment).";;
        rebeccablackos)   INFO="RebeccaBlackOS|-|https://rebeccablackos.com/|Debian-based experimental Wayland lab (VT-less kernel).";;
        tsurugi)          INFO="Tsurugi Linux|-|https://tsurugi-linux.org/|DFIR analysis and live-response Linux distribution.";;
        wifislax)         INFO="WiFiSlax|-|https://www.wifislax.com/|Slackware-based wireless auditing distro (archive.org).";;
        whonix)           INFO="Whonix|-|https://www.whonix.org/|Tor-routed privacy OS; single LXQt image used for both Gateway and Workstation VMs.";;
        9front)           INFO="9front|-|https://9front.org/|Community fork of Plan 9 from Bell Labs.";;
        kicksecure)       INFO="Kicksecure|-|https://www.kicksecure.com/|Hardened Debian-based security-focused distribution.";;
        nst)              INFO="Network Security Toolkit|-|https://www.networksecuritytoolkit.org/|Fedora-based distro for network analysis and monitoring.";;
        nyarch)           INFO="Nyarch Linux|-|https://nyarchlinux.moe/|Arch-based anime-themed distribution.";;
        pentoo)           INFO="Pentoo|-|https://www.pentoo.ch/|Gentoo-based pentesting live distribution.";;
        plan9)            INFO="Plan 9|-|https://9p.io/plan9/|Bell Labs distributed OS (historic ISOs on archive.org).";;
        ponyos)           INFO="PonyOS|-|https://ponyos.org/|Hobby Unix-like OS with its own kernel — not Linux.";;
        securityonion)    INFO="Security Onion|-|https://securityonionsolutions.com/|SOC/NIDS hunting distro with Zeek, Suricata and PCAP tools.";;
        uwuntu)           INFO="UwUntu|-|https://uwuntuos.com/|Ubuntu-based meme distribution.";;
        kubuntu)          INFO="Kubuntu|-|https://kubuntu.org/|Free, complete, and open-source alternative to Microsoft Windows and Mac OS X which contains everything you need to work, play, or share.";;
        linuxlite)        INFO="Linux Lite|-|https://www.linuxliteos.com/|Your first simple, fast and free stop in the world of Linux.";;
        linuxmint)        INFO="Linux Mint|-|https://linuxmint.com/|Designed to work out of the box and comes fully equipped with the apps most people need.";;
        lmde)             INFO="Linux Mint Debian Edition|-|https://www.linuxmint.com/download_lmde.php|Aims to be as similar as possible to Linux Mint, but without using Ubuntu. The package base is provided by Debian instead.";;
        lubuntu)          INFO="Lubuntu|-|https://lubuntu.me/|Complete Operating System that ships the essential apps and services for daily use: office applications, PDF reader,  image editor, music and video players, etc. Using lightwave lxde/lxqt.";;
        maboxlinux)       INFO="Mabox Linux|-|https://maboxlinux.org/|Lightweight, functional and easy to customize Openbox desktop";;
        midnightbsd)      INFO="MidnightBSD|-|https://www.midnightbsd.org/|FreeBSD-derived operating system for desktop users.";;
        mageia)           INFO="Mageia|-|https://www.mageia.org/|Stable, secure operating system for desktop & server.";;
        manjaro)          INFO="Manjaro|-|https://manjaro.org/|Versatile, free, and open-source Linux operating system designed with a strong focus on safeguarding user privacy and offering extensive control over hardware.";;
        mxlinux)          INFO="MX Linux|-|https://mxlinux.org/|Designed to combine elegant and efficient desktops with high stability and solid performance.";;
        netboot)          INFO="netboot.xyz|-|https://netboot.xyz/|Your favorite operating systems in one place.";;
        netbsd)           INFO="NetBSD|-|https://www.netbsd.org/|Free, fast, secure, and highly portable Unix-like Open Source operating system. It is available for a wide range of platforms, from large-scale servers and powerful desktop systems to handheld and embedded devices.";;
        nitrux)           INFO="Nitrux|-|https://nxos.org/|Powered by Debian, KDE Plasma and Frameworks, and AppImages.";;
        nobara)           INFO="Nobara|-|https://nobaraproject.org/|Fedora-based Linux distribution optimized for gaming and content creation.";;
        nixos)            INFO="NixOS|-|https://nixos.org/|Linux distribution based on Nix package manager, tool that takes a unique approach to package management and system configuration.";;
        nwg-shell)        INFO="nwg-shell|nwg:nwg|https://nwg-piotr.github.io/nwg-shell/|Arch Linux ISO with nwg-shell for sway and Hyprland";;
        omarchy)          INFO="Omarchy|-|https://omarchy.org/|Beautiful, modern & opinionated Arch-based Linux distribution featuring the Hyprland tiling window manager, created by DHH.";;
        macos)            INFO="macOS|-|https://www.apple.com/macos/|Work and play on your Mac are even more powerful. Elevate your presence on video calls. Access information in all-new ways. Boost gaming performance. And discover even more ways to personalize your Mac.";;
        openbsd)          INFO="OpenBSD|-|https://www.openbsd.org/|FREE, multi-platform 4.4BSD-based UNIX-like operating system. Our efforts emphasize portability, standardization, correctness, proactive security and integrated cryptography.";;
        openfyde)         INFO="openFyde|-|https://openfyde.io/|Open-source alternative to ChromeOS, based on Chromium OS.";;
        openindiana)      INFO="OpenIndiana|-|https://www.openindiana.org/|Community supported illumos-based operating system.";;
        openmediavault)   INFO="openmediavault|-|https://www.openmediavault.org/|Debian-based network attached storage (NAS) solution.";;
        openmandriva)     INFO="OpenMandriva Lx|-|https://www.openmandriva.org/|Community-driven Linux distribution with KDE Plasma.";;
        opnsense)         INFO="OPNsense|-|https://opnsense.org/|FreeBSD-based open source firewall and routing platform.";;
        opensuse)         INFO="openSUSE|-|https://www.opensuse.org/|The makers choice for sysadmins, developers and desktop users.";;
        openkylin)        INFO="openKylin|-|https://www.openkylin.top/|Chinese national OS, open community of Kylin.";;
        pardus)           INFO="Pardus|-|https://www.pardus.org.tr/|Turkish national OS for education and public administration.";;
        oraclelinux)      INFO="Oracle Linux|-|https://www.oracle.com/linux/|Linux with everything required to deploy, optimize, and manage applications on-premises, in the cloud, and at the edge.";;
        parrotsec)        INFO="Parrot Security|parrot:parrot|https://www.parrotsec.org/|Provides a huge arsenal of tools, utilities and libraries that IT and security professionals can use to test and assess the security of their assets in a reliable, compliant and reproducible way.";;
        pclinuxos)        INFO="PCLinuxOS|-|https://www.pclinuxos.com/|PCLinuxOS is a free easy to use Linux-based Operating System for x86_64 desktops or laptops.";;
        peppermint)       INFO="PeppermintOS|-|https://peppermintos.com/|Provides a user with the opportunity to build the system that best fits their needs. While at the same time providing a functioning OS with minimum hassle out of the box.";;
        popos)            INFO="Pop!_OS|-|https://pop.system76.com/|Operating system for STEM and creative professionals who use their computer as a tool to discover and create.";;
        porteus)          INFO="Porteus|-|http://www.porteus.org/|Complete linux operating system that is optimized to run from CD, USB flash drive, hard drive, or other bootable storage media.";;
        primtux)          INFO="PrimTux|-|https://primtux.fr/|A complete and customizable GNU/Linux operating system intended for primary school students and suitable even for older hardware.";;
        q4os)             INFO="Q4OS|-|https://q4os.org/|Lightweight Debian-based distribution with Trinity and KDE desktops.";;
        proxmox-ve)       INFO="Proxmox VE|-|https://proxmox.com/en/proxmox-virtual-environment/|Proxmox Virtual Environment is a complete, open-source server management platform for enterprise virtualization.";;
        pureos)           INFO="PureOS|-|https://www.pureos.net/|A fully free/libre and open source GNU/Linux operating system, endorsed by the Free Software Foundation.";;
        qubes)            INFO="Qubes OS|-|https://www.qubes-os.org/|A reasonably secure operating system that uses Xen-based virtualization to provide strong isolation.";;
        reactos)          INFO="ReactOS|-|https://reactos.org/|Imagine running your favorite Windows applications and drivers in an open-source environment you can trust.";;
        redoxos)          INFO="Redox OS|-|https://www.redox-os.org/|Unix-like microkernel operating system written in Rust, with a focus on safety and correctness.";;
        rebornos)         INFO="RebornOS|-|https://rebornos.org/|Aiming to make Arch Linux as user friendly as possible by providing interface solutions to things you normally have to do in a terminal.";;
        rockylinux)       INFO="Rocky Linux|-|https://rockylinux.org/|Open-source enterprise operating system designed to be 100% bug-for-bug compatible with Red Hat Enterprise Linux®.";;
        siduction)        INFO="Siduction|-|https://siduction.org/|Operating system based on the Linux kernel and the GNU project. In addition, there are applications and libraries from Debian.";;
        slackware)        INFO="Slackware|-|http://www.slackware.com/|Advanced Linux operating system, designed with the twin goals of ease of use and stability as top priorities.";;
        slax)             INFO="Slax|-|https://www.slax.org/|Compact, fast, and modern Linux operating system that combines sleek design with modular approach. With the ability to run directly from a USB flash drive without the need for installation, Slax is truly portable and fits easily in your pocket.";;
        slint)            INFO="Slint|-|https://slint.fr/|Slint is an easy-to-use, versatile, blind-friendly Linux distribution for 64-bit computers. Slint is based on Slackware and borrows tools from Salix. Maintainer: Didier Spaier.";;
        slitaz)           INFO="SliTaz|-|https://www.slitaz.org/en/|Simple, fast and low resource Linux OS for servers & desktops.";;
        solus)            INFO="Solus|-|https://getsol.us/|Designed for home computing. Every tweak enables us to deliver a cohesive computing experience.";;
        sparkylinux)      INFO="SparkyLinux|-|https://sparkylinux.org/|Fast, lightweight and fully customizable operating system which offers several versions for different use cases.";;
        spirallinux)      INFO="SpiralLinux|-|https://spirallinux.github.io/|Selection of Linux spins built from Debian GNU/Linux, with a focus on simplicity and out-of-the-box usability across all the major desktop environments.";;
        tails)            INFO="Tails|-|https://tails.net/|Portable operating system that protects against surveillance and censorship.";;
        systemrescue)     INFO="SystemRescue|-|https://www.system-rescue.org/|Arch-based rescue system for repairing unbootable computers.";;
        templeos)         INFO="TempleOS|-|https://templeos.org/|TempleOS is a public domain, open source, ring-0-only operating system.";;
        tinycore)         INFO="Tiny Core Linux|-|http://www.tinycorelinux.net/|Highly modular based system with community build extensions.";;
        trisquel)         INFO="Trisquel|-|https://trisquel.info/|Fully free operating system for home users, small enterprises and educational centers.";;
        truenas-scale)    INFO="TrueNAS SCALE|-|https://www.truenas.com/truenas-scale/|Debian-based NAS operating system by iXsystems.";;
        tuxedo-os)        INFO="Tuxedo OS|-|https://www.tuxedocomputers.com/en/|KDE Ubuntu LTS designed to go with their Linux hardware.";;
        ubuntu)           INFO="Ubuntu|-|https://ubuntu.com/|Complete desktop Linux operating system, freely available with both community and professional support.";;
        ubuntu-budgie)    INFO="Ubuntu Budgie|-|https://ubuntubudgie.org/|Community developed distribution, integrating the Budgie Desktop Environment with Ubuntu at its core.";;
        ubuntucinnamon)   INFO="Ubuntu Cinnamon|-|https://ubuntucinnamon.org/|Community-driven, featuring Linux Mint’s Cinnamon Desktop with Ubuntu at the core, packed fast and full of features, here is the most traditionally modern desktop you will ever love.";;
        ubuntukylin)      INFO="Ubuntu Kylin|-|https://ubuntukylin.com/|Universal desktop operating system for personal computers, laptops, and embedded devices. It is dedicated to bringing a smarter user experience to users all over the world.";;
        ubuntu-mate)      INFO="Ubuntu MATE|-|https://ubuntu-mate.org/|Stable, easy-to-use operating system with a configurable desktop environment. It is ideal for those who want the most out of their computers and prefer a traditional desktop metaphor. Using Mate desktop.";;
        ubuntu-server)    INFO="Ubuntu Server|-|https://ubuntu.com/server|Brings economic and technical scalability to your datacentre, public or private. Whether you want to deploy an OpenStack cloud, a Kubernetes cluster or a 50,000-node render farm, Ubuntu Server delivers the best value scale-out performance available.";;
        ubuntustudio)     INFO="Ubuntu Studio|-|https://ubuntustudio.org/|Comes preinstalled with a selection of the most common free multimedia applications available, and is configured for best performance for various purposes: Audio, Graphics, Video, Photography and Publishing.";;
        ubuntu-unity)     INFO="Ubuntu Unity|-|https://ubuntuunity.org/|Flavor of Ubuntu featuring the Unity7 desktop environment (the default desktop environment used by Ubuntu from 2010-2017).";;
        vanillaos)        INFO="Vanilla OS|-|https://vanillaos.org/|Designed to be a reliable and productive operating system for your daily work.";;
        vyos)             INFO="VyOS|-|https://vyos.io/|Open source network operating system for routers and firewalls.";;
        void)             INFO="Void Linux|anon:voidlinux|https://voidlinux.org/|General purpose operating system. Its package system allows you to quickly install, update and remove software; software is provided in binary packages or can be built directly from sources.";;
        windows)          INFO="Windows|-|https://www.microsoft.com/en-us/windows/|Whether you’re gaming, studying, running a business, or running a household, Windows helps you get it done.";;
        windows-server)   INFO="Windows Server|-|https://www.microsoft.com/en-us/windows-server/|Platform for building an infrastructure of connected applications, networks, and web services.";;
        xubuntu)          INFO="Xubuntu|-|https://xubuntu.org/|Elegant and easy to use operating system. Xubuntu comes with Xfce, which is a stable, light and configurable desktop environment.";;
        zorin)            INFO="Zorin OS|-|https://zorin.com/os/|Alternative to Windows and macOS designed to make your computer faster, more powerful, secure, and privacy-respecting.";;
    esac
    echo "${INFO}"
}


function show_os_info() {
    echo
    echo -e "$(os_info "${1}" | cut -d'|' -f 1)"
    echo -e " - Credentials:\t$(os_info "${1}" | cut -d'|' -f 2)"
    echo -e " - Website:\t$(os_info "${1}" | cut -d'|' -f 3)"
    echo -e " - Description:\t$(os_info "${1}" | cut -d'|' -f 4)"
}


function pretty_name() {
    os_info "${1}" | cut -d'|' -f 1
}


# Just in case quickget want use it
function os_homepage() {
    os_info "${1}" | cut -d'|' -f 3
}


function os_support() {
    echo 9front \
    alma \
    backbox \
    biebian \
    caine \
    canaima \
    altlinux \
    alpine \
    amogos \
    android \
    antix \
    archcraft \
    archlinux \
    artixlinux \
    azurelinux \
    batocera \
    bazzite \
    biglinux \
    blackarch \
    blendos \
    blissos \
    bodhi \
    bunsenlabs \
    cachyos \
    centos-stream \
    chimeralinux \
    crunchbang++ \
    debian \
    deepin \
    devuan \
    dragonflybsd \
    dragonos \
    easyos \
    edubuntu \
    elementary \
    endeavouros \
    endless \
    esfera \
    fedora \
    fedoracore \
    freebsd \
    freedos \
    garuda \
    gecos \
    gentoo \
    ghostbsd \
    gnomeos \
    gnulinex \
    gobolinux \
    guadalinex \
    guadalinexedu \
    educaandos \
    emmabuntus \
    guix \
    haiku \
    hannahmontana \
    kali \
    kaos \
    kdelinux \
    kdeneon \
    kicksecure \
    kolibrios \
    kubuntu \
    linuxlite \
    linuxmint \
    lliurex \
    linkat \
    lmde \
    lubuntu \
    maboxlinux \
    macos \
    mageia \
    mangaka \
    manjaro \
    maxlinux \
    molinux \
    minino \
    midnightbsd \
    mxlinux \
    netboot \
    netbsd \
    nitrux \
    nixos \
    nobara \
    nst \
    nwg-shell \
    nyarch \
    omarchy \
    openbsd \
    openfyde \
    openindiana \
    openkylin \
    openmandriva \
    openmediavault \
    opensuse \
    opnsense \
    oraclelinux \
    os2warp \
    pardus \
    parrotsec \
    pclinuxos \
    pentoo \
    peppermint \
    plan9 \
    ponyos \
    popos \
    porteus \
    primtux \
    proxmox-ve \
    pureos \
    q4os \
    qubes \
    reactos \
    rebeccablackos \
    rebornos \
    recreomatic \
    redoxos \
    rockylinux \
    securityonion \
    siduction \
    slackware \
    slax \
    slint \
    slitaz \
    solus \
    sparkylinux \
    spirallinux \
    subgraph \
    syllable \
    systemrescue \
    tails \
    templeos \
    tinycore \
    trisquel \
    truenas-scale \
    tsurugi \
    tuxedo-os \
    ubuntu \
    ubuntu-budgie \
    ubuntu-mate \
    ubuntu-server \
    ubuntu-unity \
    ubuntucinnamon \
    ubuntukylin \
    ubuntustudio \
    uwuntu \
    vanillaos \
    vitalinux \
    void \
    vyos \
    whonix \
    wifislax \
    windows \
    windows-server \
    xubuntu \
    zorin
    }
