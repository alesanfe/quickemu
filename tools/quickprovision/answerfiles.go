// answerfiles.go: the unattended-install payload renderers. Each
// provider's answer file is generated text built from the profile —
// the orchestrator never embeds per-OS logic, these functions are
// where that logic lives.
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// xmlEscape guards the profile values embedded in autounattend.xml.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;",
		`"`, "&quot;", `'`, "&apos;")
	return r.Replace(s)
}

// winPassword resolves the account password for the unattend file —
// Windows (and especially Server) enforces complexity, so an empty
// profile password gets a compliant default rather than a weak one
// that would make the specialize pass fail silently.
func winPassword(p *Profile) string {
	if p.Identity.Password != "" {
		return p.Identity.Password
	}
	return "Quickemu#2026"
}

// winCulture maps a POSIX lang ("es_ES", "en_GB") to the
// (culture, langID) pair the unattend template wants. Falls back to
// en-US for anything unmapped.
func winCulture(lang string) (culture, langID string) {
	m := map[string][2]string{
		"en_gb": {"en-GB", "0809"}, "es_es": {"es-ES", "0c0a"},
		"es_mx": {"es-MX", "080a"}, "fr_fr": {"fr-FR", "040c"},
		"de_de": {"de-DE", "0407"}, "it_it": {"it-IT", "0410"},
		"pt_pt": {"pt-PT", "0816"}, "pt_br": {"pt-BR", "0416"},
		"nl_nl": {"nl-NL", "0413"}, "ja_jp": {"ja-JP", "0411"},
		"ko_kr": {"ko-KR", "0412"}, "zh_cn": {"zh-CN", "0804"},
	}
	k := strings.ToLower(strings.SplitN(lang, ".", 2)[0])
	if c, ok := m[k]; ok {
		return c[0], c[1]
	}
	return "en-US", "0409"
}

// winDriverDir maps the catalog os/release to the virtio-win driver
// directory flavour used in the unattend DriverPaths.
func winDriverDir(osName, release string) string {
	if osName == "windows-server" {
		switch release {
		case "2025":
			return "w2k25"
		case "2022":
			return "w2k22"
		case "2019":
			return "w2k19"
		default:
			return "w2k16"
		}
	}
	if release == "10" {
		return "w10"
	}
	return "w11"
}

// renderAutounattend adapts quickget-vm's proven template: profile
// identity/locale substituted, virtio DriverPaths kept (the installer
// disk is virtio — without these the setup sees no target disk), and
// FirstLogonCommands extended to bring up OpenSSH before powering
// off — the qemu exit is the run's install-complete signal.
func renderAutounattend(p *Profile, pubkey string) string {
	culture, langID := winCulture(p.Locale.Lang)
	drv := winDriverDir(p.System.OS, p.System.Release)
	user := xmlEscape(p.Identity.Username)
	pass := xmlEscape(winPassword(p))
	host := xmlEscape(p.Identity.Hostname)
	pubkey = strings.TrimSpace(pubkey)

	qxldod := ""
	if drv == "w10" {
		qxldod = `        <PathAndCredentials wcm:action="add" wcm:keyValue="6">
          <Path>E:\qxldod\w10\amd64</Path>
        </PathAndCredentials>
`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend"
  xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <settings pass="offlineServicing">
    <component name="Microsoft-Windows-LUA-Settings" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <EnableLUA>false</EnableLUA>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <ComputerName>` + host + `</ComputerName>
    </component>
  </settings>
  <settings pass="generalize">
    <component name="Microsoft-Windows-PnPSysprep" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <PersistAllDeviceInstalls>true</PersistAllDeviceInstalls>
    </component>
    <component name="Microsoft-Windows-Security-SPP" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <SkipRearm>1</SkipRearm>
    </component>
  </settings>
  <settings pass="specialize">
    <component name="Microsoft-Windows-Security-SPP-UX" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <SkipAutoActivation>true</SkipAutoActivation>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <ComputerName>` + host + `</ComputerName>
      <ProductKey>` + winProductKey(p.System.OS, p.System.Release) + `</ProductKey>
    </component>
    <component name="Microsoft-Windows-SQMApi" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <CEIPEnabled>0</CEIPEnabled>
    </component>
  </settings>
  <settings pass="windowsPE">
    <component name="Microsoft-Windows-International-Core-WinPE" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <SetupUILanguage>
        <UILanguage>` + culture + `</UILanguage>
      </SetupUILanguage>
      <InputLocale>` + langID + `:0000` + langID + `</InputLocale>
      <SystemLocale>` + culture + `</SystemLocale>
      <UILanguage>` + culture + `</UILanguage>
      <UserLocale>` + culture + `</UserLocale>
    </component>
    <component name="Microsoft-Windows-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <Diagnostics>
        <OptIn>false</OptIn>
      </Diagnostics>
      <DiskConfiguration>
        <Disk wcm:action="add">
          <DiskID>0</DiskID>
          <WillWipeDisk>true</WillWipeDisk>
          <CreatePartitions>
            <CreatePartition wcm:action="add">
              <Order>1</Order>
              <Type>Primary</Type>
              <Size>256</Size>
            </CreatePartition>
            <CreatePartition wcm:action="add">
              <Order>2</Order>
              <Type>EFI</Type>
              <Size>128</Size>
            </CreatePartition>
            <CreatePartition wcm:action="add">
              <Order>3</Order>
              <Type>MSR</Type>
              <Size>128</Size>
            </CreatePartition>
            <CreatePartition wcm:action="add">
              <Order>4</Order>
              <Type>Primary</Type>
              <Extend>true</Extend>
            </CreatePartition>
          </CreatePartitions>
          <ModifyPartitions>
            <ModifyPartition wcm:action="add">
              <Order>1</Order>
              <PartitionID>1</PartitionID>
              <Label>WINRE</Label>
              <Format>NTFS</Format>
              <TypeID>DE94BBA4-06D1-4D40-A16A-BFD50179D6AC</TypeID>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
              <Order>2</Order>
              <PartitionID>2</PartitionID>
              <Label>System</Label>
              <Format>FAT32</Format>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
              <Order>3</Order>
              <PartitionID>3</PartitionID>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
              <Order>4</Order>
              <PartitionID>4</PartitionID>
              <Label>Windows</Label>
              <Letter>C</Letter>
              <Format>NTFS</Format>
            </ModifyPartition>
          </ModifyPartitions>
        </Disk>
      </DiskConfiguration>
      <DynamicUpdate>
        <Enable>true</Enable>
        <WillShowUI>Never</WillShowUI>
      </DynamicUpdate>
      <ImageInstall>
        <OSImage>
          <InstallTo>
            <DiskID>0</DiskID>
            <PartitionID>4</PartitionID>
          </InstallTo>
          <InstallToAvailablePartition>false</InstallToAvailablePartition>
        </OSImage>
      </ImageInstall>
      <RunSynchronous>
        <RunSynchronousCommand wcm:action="add">
          <Order>1</Order>
          <Path>reg add HKLM\System\Setup\LabConfig /v BypassCPUCheck /t REG_DWORD /d 0x00000001 /f</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>2</Order>
          <Path>reg add HKLM\System\Setup\LabConfig /v BypassRAMCheck /t REG_DWORD /d 0x00000001 /f</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>3</Order>
          <Path>reg add HKLM\System\Setup\LabConfig /v BypassSecureBootCheck /t REG_DWORD /d 0x00000001 /f</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>4</Order>
          <Path>reg add HKLM\System\Setup\LabConfig /v BypassTPMCheck /t REG_DWORD /d 0x00000001 /f</Path>
        </RunSynchronousCommand>
      </RunSynchronous>
      <UpgradeData>
        <Upgrade>false</Upgrade>
        <WillShowUI>Never</WillShowUI>
      </UpgradeData>
      <UserData>
        <AcceptEula>true</AcceptEula>
        <FullName>` + user + `</FullName>
        <Organization>Quickemu Project</Organization>
        <ProductKey>
          <Key>` + winProductKey(p.System.OS, p.System.Release) + `</Key>
          <WillShowUI>Never</WillShowUI>
        </ProductKey>
      </UserData>
    </component>
    <component name="Microsoft-Windows-PnpCustomizationsWinPE" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" processorArchitecture="amd64" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <DriverPaths>
        <PathAndCredentials wcm:action="add" wcm:keyValue="1">
          <Path>E:\qemufwcfg\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="2">
          <Path>E:\vioinput\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="3">
          <Path>E:\vioscsi\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="4">
          <Path>E:\viostor\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="5">
          <Path>E:\vioserial\` + drv + `\amd64</Path>
        </PathAndCredentials>
` + qxldod + `        <PathAndCredentials wcm:action="add" wcm:keyValue="7">
          <Path>E:\viorng\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="8">
          <Path>E:\NetKVM\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="9">
          <Path>E:\viofs\` + drv + `\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="10">
          <Path>E:\Balloon\` + drv + `\amd64</Path>
        </PathAndCredentials>
      </DriverPaths>
    </component>
  </settings>
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <AutoLogon>
        <Password>
          <Value>` + pass + `</Value>
          <PlainText>true</PlainText>
        </Password>
        <Enabled>true</Enabled>
        <Username>` + user + `</Username>
      </AutoLogon>
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideLocalAccountScreen>true</HideLocalAccountScreen>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <NetworkLocation>Home</NetworkLocation>
        <ProtectYourPC>3</ProtectYourPC>
        <SkipUserOOBE>true</SkipUserOOBE>
        <SkipMachineOOBE>true</SkipMachineOOBE>
        <VMModeOptimizations>
          <SkipWinREInitialization>true</SkipWinREInitialization>
        </VMModeOptimizations>
      </OOBE>
      <UserAccounts>
        <LocalAccounts>
          <LocalAccount wcm:action="add">
            <Password>
              <Value>` + pass + `</Value>
              <PlainText>true</PlainText>
            </Password>
            <Description>` + user + `</Description>
            <DisplayName>` + user + `</DisplayName>
            <Group>Administrators</Group>
            <Name>` + user + `</Name>
          </LocalAccount>
        </LocalAccounts>
      </UserAccounts>
      <FirstLogonCommands>
        <SynchronousCommand wcm:action="add">
          <CommandLine>msiexec /i E:\guest-agent\qemu-ga-x86_64.msi /quiet /passive /qn</CommandLine>
          <Description>Install Virtio Guest Agent</Description>
          <Order>1</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0"</CommandLine>
          <Description>Install OpenSSH Server</Description>
          <Order>2</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>powershell -NoProfile -Command "New-NetFirewallRule -Name sshd -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null"</CommandLine>
          <Description>Allow SSH through the firewall</Description>
          <Order>3</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>powershell -NoProfile -Command "Set-Service sshd -StartupType Automatic; Start-Service sshd"</CommandLine>
          <Description>Enable and start sshd</Description>
          <Order>4</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>reg add "HKLM\SOFTWARE\OpenSSH" /v DefaultShell /t REG_SZ /d "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe" /f</CommandLine>
          <Description>PowerShell as the SSH default shell</Description>
          <Order>5</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>powershell -NoProfile -Command "$p='C:\ProgramData\ssh\administrators_authorized_keys'; Set-Content -Path $p -Value '` + pubkey + `' -Encoding ascii; icacls $p /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F' | Out-Null"</CommandLine>
          <Description>Install the run's SSH authorized key</Description>
          <Order>6</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>cmd /c net accounts /maxpwage:unlimited</CommandLine>
          <Description>Local account passwords never expire</Description>
          <Order>7</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>shutdown /s /t 5 /f</CommandLine>
          <Description>Power off — the run's install-complete signal</Description>
          <Order>8</Order>
        </SynchronousCommand>
      </FirstLogonCommands>
    </component>
  </settings>
</unattend>
`
}

// winProductKey picks the KMS client-setup key for the edition —
// these are Microsoft's documented GVLK keys (no activation, they
// only let setup proceed; the user activates their own licence).
func winProductKey(osName, release string) string {
	if osName == "windows-server" {
		switch release {
		case "2025", "2022":
			return "WX4NM-KYWYW-QJJR4-XV3QB-6VM33" // Server Standard
		case "2019":
			return "N69G4-B89J2-4G8F4-WWYCC-J464C"
		default:
			return "WC2BQ-8NRM3-FDDYY-2BFGV-KHKQY" // 2016 Standard
		}
	}
	if release == "10" {
		return "W269N-WFGWX-YVC9B-4J6C9-T83GX" // Win10 Pro KMS
	}
	return "W269N-WFGWX-YVC9B-4J6C9-T83GX" // Win11 Pro KMS
}

// --- Debian-installer preseed (debian, kali) ------------------------

// debianSuite maps a catalog release to the archive codename the
// preseed mirror keys need.
func debianSuite(release string) string {
	switch {
	case strings.HasPrefix(release, "13"):
		return "trixie"
	case strings.HasPrefix(release, "12"):
		return "bookworm"
	case strings.HasPrefix(release, "11"):
		return "bullseye"
	}
	return "stable"
}

// trisquelSuite maps a trisquel release to its codename (the catalog
// carries numeric releases; d-i wants the suite name).
func trisquelSuite(release string) string {
	switch {
	case strings.HasPrefix(release, "12"):
		return "ecne"
	case strings.HasPrefix(release, "11"):
		return "aramis"
	case strings.HasPrefix(release, "10"):
		return "nabia"
	}
	return "aramis"
}

// renderPreseed writes a fully-automatic d-i preseed. It leans on the
// same contract as the autoinstall flow: the last d-i action powers
// the guest off, so the end of install is a qemu exit.
func renderPreseed(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	lang := p.Locale.Lang
	if lang == "" {
		lang = "en_US.UTF-8"
	}
	kbd := p.Locale.Keyboard
	if kbd == "" {
		kbd = "us"
	}
	mirrorHost, mirrorDir, suite := "deb.debian.org", "/debian", debianSuite(p.System.Release)
	// eduProfile is the debian-edu metapackage question — without an
	// answer the installer stops for it even with priority=critical.
	eduProfile := ""
	switch p.System.OS {
	case "kali":
		mirrorHost, mirrorDir, suite = "http.kali.org", "/kali", "kali-rolling"
	case "devuan":
		// Devuan's catalog releases are already suite names
		// (daedalus/excalibur/chimaera) and it merges Debian repos.
		mirrorHost, mirrorDir, suite = "deb.devuan.org", "/merged", p.System.Release
	case "trisquel":
		mirrorHost, mirrorDir, suite = "archive.trisquel.info", "/trisquel",
			trisquelSuite(p.System.Release)
	case "debian-edu":
		eduProfile = "d-i debian-edu-install/profile select Minimal\n"
	}
	var k strings.Builder
	for _, key := range keys {
		k.WriteString(strings.TrimSpace(key) + " ")
	}
	return `d-i debian-installer/locale string ` + lang + `
d-i keyboard-configuration/xkb-keymap select ` + kbd + `
d-i netcfg/get_hostname string ` + host + `
d-i netcfg/get_domain string local
d-i netcfg/choose_interface select auto
d-i mirror/country string manual
d-i mirror/http/hostname string ` + mirrorHost + `
d-i mirror/http/directory string ` + mirrorDir + `
d-i mirror/suite string ` + suite + `
d-i mirror/codename string ` + suite + `
` + eduProfile + `d-i apt-setup/non-free-firmware boolean true
d-i apt-setup/contrib boolean true
d-i apt-setup/non-free boolean true
d-i clock-setup/utc boolean true
d-i time/zone string ` + tz + `
d-i passwd/root-login boolean false
d-i passwd/user-fullname string ` + user + `
d-i passwd/username string ` + user + `
d-i passwd/user-password-crypted password ` + hash + `
d-i user-setup/allow-password-weak boolean true
d-i partman-auto/method string regular
d-i partman-auto/choose_recipe select atomic
d-i partman/confirm_write_new_label boolean true
d-i partman/choose_partition select finish
d-i partman/confirm boolean true
d-i partman/confirm_nooverwrite boolean true
d-i pkgsel/include string openssh-server sudo curl qemu-guest-agent
d-i pkgsel/upgrade select safe-upgrade
d-i grub-installer/only_debian boolean true
d-i grub-installer/with_other_os boolean true
d-i finish-install/reboot_in_progress note
d-i debian-installer/exit/poweroff boolean true
d-i preseed/late_command string in-target sh -c 'echo "` + user + ` ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/` + user + ` && chmod 440 /etc/sudoers.d/` + user + ` && mkdir -p /home/` + user + `/.ssh && echo "` + strings.TrimSpace(k.String()) + `" > /home/` + user + `/.ssh/authorized_keys && chmod 700 /home/` + user + `/.ssh && chmod 600 /home/` + user + `/.ssh/authorized_keys && chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh'
`
}

// --- Anaconda kickstart (fedora Server, alma, rocky, centos, oracle, tencentos) ---

// renderKickstart writes a minimal-but-complete anaconda answer file.
// The `poweroff` directive gives the same completion contract as the
// other providers: end of install is a qemu exit. The wheel group is
// the sudo mechanism across the RHEL family; the run user gets it
// plus NOPASSWD via %post.
func renderKickstart(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	lang := p.Locale.Lang
	if lang == "" {
		lang = "en_US.UTF-8"
	}
	kbd := p.Locale.Keyboard
	if kbd == "" {
		kbd = "us"
	}
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	var sb strings.Builder
	sb.WriteString("text\n")
	fmt.Fprintf(&sb, "lang %s\nkeyboard %s\ntimezone %s --utc\n", lang, kbd, tz)
	fmt.Fprintf(&sb, "network --bootproto=dhcp --device=link --activate --hostname=%s\n", host)
	// `cdrom` sources packages from the media — only DVDs carry them.
	// Netinst editions (boot/netinst ISOs) omit the source command so
	// anaconda falls back to the product's default remote mirror.
	switch strings.ToLower(p.System.Edition) {
	case "boot", "netinst", "netboot", "network":
	default:
		sb.WriteString("cdrom\n")
	}
	sb.WriteString("zerombr\nclearpart --all --initlabel\nautopart --type=plain\n")
	sb.WriteString("bootloader --location=mbr\n")
	sb.WriteString("rootpw --lock\n")
	fmt.Fprintf(&sb, "user --name=%s --password=%s --iscrypted --groups=wheel\n", user, hash)
	for _, k := range keys {
		fmt.Fprintf(&sb, "sshkey --username=%s %s\n", user, yamlQ(strings.TrimSpace(k)))
	}
	sb.WriteString("selinux --permissive\n") // qemu-guest-agent + NOPASSWD sudo stay simple
	sb.WriteString("firewall --service=ssh\n")
	sb.WriteString("services --enabled=\"sshd qemu-guest-agent\"\n")
	sb.WriteString("poweroff\n")
	sb.WriteString("%packages\n@core\nopenssh-server\nsudo\nqemu-guest-agent\ncurl\n%end\n")
	fmt.Fprintf(&sb, "%%post --nochroot\nmkdir -p /mnt/sysimage/etc/sudoers.d\n"+
		"echo '%s ALL=(ALL) NOPASSWD:ALL' > /mnt/sysimage/etc/sudoers.d/%s\n"+
		"chmod 440 /mnt/sysimage/etc/sudoers.d/%s\n%%end\n", user, user, user)
	return sb.String()
}

// --- openSUSE AutoYaST ----------------------------------------------

// renderAutoyast writes a minimal AutoYaST profile. <final_halt>
// gives the same poweroff completion contract as the other providers
// (supported since SLE 15 SP3 / Leap 15.3 — all cataloged releases).
// The firewall is disabled outright rather than punched for ssh —
// these are throwaway automation VMs on slirp.
func renderAutoyast(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	// YaST wants "es_ES" not "es_ES.UTF-8".
	lang := strings.SplitN(p.Locale.Lang, ".", 2)[0]
	if lang == "" {
		lang = "en_US"
	}
	kbd := p.Locale.Keyboard
	if kbd == "" {
		kbd = "us"
	}
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	var keysXML strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&keysXML, "        <listentry>%s</listentry>\n",
			strings.TrimSpace(k))
	}
	return `<?xml version="1.0"?>
<!DOCTYPE profile>
<profile xmlns="http://www.suse.com/1.0/yast2ns" xmlns:config="http://www.suse.com/1.0/configns">
  <general>
    <mode>
      <confirm config:type="boolean">false</confirm>
      <final_halt config:type="boolean">true</final_halt>
      <second_stage config:type="boolean">true</second_stage>
    </mode>
  </general>
  <keyboard><keymap>` + kbd + `</keymap></keyboard>
  <language><language>` + lang + `</language></language>
  <timezone><timezone>` + tz + `</timezone><hwclock>UTC</hwclock></timezone>
  <networking>
    <keep_install_network config:type="boolean">true</keep_install_network>
    <hostname>` + host + `</hostname>
  </networking>
  <partitioning config:type="list">
    <drive>
      <device>/dev/vda</device>
      <initialize config:type="boolean">true</initialize>
      <use>all</use>
    </drive>
  </partitioning>
  <software>
    <patterns config:type="list">
      <pattern>base</pattern>
      <pattern>minimal_base</pattern>
    </patterns>
    <packages config:type="list">
      <package>openssh</package>
      <package>sudo</package>
      <package>qemu-guest-agent</package>
      <package>curl</package>
    </packages>
  </software>
  <users config:type="list">
    <user>
      <username>root</username>
      <user_password>!</user_password>
      <encrypted config:type="boolean">true</encrypted>
    </user>
    <user>
      <username>` + user + `</username>
      <user_password>` + hash + `</user_password>
      <encrypted config:type="boolean">true</encrypted>
      <fullname>` + user + `</fullname>
      <shell>/bin/bash</shell>
      <authorized_keys config:type="list">
` + keysXML.String() + `      </authorized_keys>
    </user>
  </users>
  <services-manager>
    <default_target>multi-user</default_target>
    <services>
      <enable config:type="list">
        <service>sshd</service>
        <service>qemu-guest-agent</service>
      </enable>
    </services>
  </services-manager>
  <firewall>
    <enable_firewall config:type="boolean">false</enable_firewall>
    <start_firewall config:type="boolean">false</start_firewall>
  </firewall>
  <scripts>
    <post-scripts config:type="list">
      <script>
        <filename>sudo.sh</filename>
        <interpreter>shell</interpreter>
        <source><![CDATA[#!/bin/sh
echo '` + user + ` ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/` + user + `
chmod 440 /etc/sudoers.d/` + user + `
]]></source>
      </script>
    </post-scripts>
  </scripts>
</profile>
`
}

// --- Arch Linux pacstrap script -------------------------------------

// renderArchInstall writes the script the serial console fetches and
// runs inside the live ISO. pacstrap rather than archinstall: no
// config schema to chase across releases. Boot firmware is detected
// at runtime — bootctl for EFI guests, grub+msdos for SeaBIOS. The
// trailing poweroff is the completion signal; a failure before it
// leaves the run to time out with the serial log as evidence.
func renderArchInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	locale := p.Locale.Lang
	if locale == "" {
		locale = "en_US.UTF-8"
	}
	return `#!/bin/sh
set -e
# one bootable ext4 partition across the disk
printf 'label: dos\n, , L, *\n' | sfdisk /dev/vda
mkfs.ext4 -F /dev/vda1
mount /dev/vda1 /mnt
pacstrap -K /mnt base linux linux-firmware openssh sudo dhcpcd grub
genfstab -U /mnt >> /mnt/etc/fstab
# firmware-aware bootloader: bootctl on EFI, grub on SeaBIOS
if [ -d /sys/firmware/efi ]; then
  arch-chroot /mnt bootctl --path=/boot install
  printf 'default arch\ntimeout 3\n' > /mnt/boot/loader/loader.conf
else
  arch-chroot /mnt grub-install --target=i386-pc /dev/vda
  arch-chroot /mnt grub-mkconfig -o /boot/grub/grub.cfg
fi
echo ` + host + ` > /mnt/etc/hostname
ln -sf /usr/share/zoneinfo/` + tz + ` /mnt/etc/localtime
sed -i 's/^#\(` + locale + `\)/\1/' /mnt/etc/locale.gen
arch-chroot /mnt locale-gen
echo 'LANG=` + locale + `' > /mnt/etc/locale.conf
arch-chroot /mnt useradd -m -G wheel -s /bin/bash ` + user + `
arch-chroot /mnt usermod -p '` + hash + `' ` + user + `
mkdir -p /mnt/etc/sudoers.d /mnt/home/` + user + `/.ssh
echo '` + user + ` ALL=(ALL) NOPASSWD:ALL' > /mnt/etc/sudoers.d/` + user + `
chmod 440 /mnt/etc/sudoers.d/` + user + `
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
arch-chroot /mnt chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh
systemctl --root=/mnt enable sshd dhcpcd
poweroff
`
}

// joinKeys renders authorized_keys content for the heredoc.
func joinKeys(keys []string) string {
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(strings.TrimSpace(k) + "\n")
	}
	return b.String()
}

// --- Proxmox unattended (VE, PBS, PMG, PDM) ---------------------------

// renderProxmoxAnswer writes the answer.toml the proxmox installer
// scans for on a PROXMOX_ANSWER-labelled volume — no kernel override
// needed, the ISO's own bootloader finds it. Completion is the
// post-installation-webhook POST to /done; reboot-mode=poweroff is
// the fallback for versions that honour it.
func renderProxmoxAnswer(p *Profile, hash string, keys []string, port int) string {
	kbd := p.Locale.Keyboard
	if kbd == "" {
		kbd = "us"
	}
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	// country steers the debian mirror pool — take it from the locale.
	country := "us"
	if parts := strings.SplitN(p.Locale.Lang, "_", 2); len(parts) == 2 {
		country = strings.ToLower(strings.SplitN(parts[1], ".", 2)[0])
	}
	var sb strings.Builder
	sb.WriteString("[global]\n")
	fmt.Fprintf(&sb, "keyboard = %q\ncountry = %q\ntimezone = %q\n", kbd, country, tz)
	sb.WriteString("mailto = \"root@localhost\"\n")
	fmt.Fprintf(&sb, "root-password-hashed = %q\n", hash)
	sb.WriteString("root-ssh-keys = [\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "  %q,\n", strings.TrimSpace(k))
	}
	sb.WriteString("]\n")
	sb.WriteString("reboot-mode = \"poweroff\"\n")
	sb.WriteString("\n[network]\nsource = \"from-dhcp\"\n")
	sb.WriteString("\n[disk-setup]\nfilesystem = \"ext4\"\ndisk-list = [\"vda\"]\n")
	sb.WriteString("\n[first-boot]\n")
	sb.WriteString("source = \"from-url\"\nordering = \"after-network\"\n")
	fmt.Fprintf(&sb, "url = \"http://10.0.2.2:%d/firstboot.sh\"\n", port)
	sb.WriteString("\n[post-installation-webhook]\n")
	fmt.Fprintf(&sb, "url = \"http://10.0.2.2:%d/done\"\n", port)
	return sb.String()
}

// renderProxmoxFirstboot is the script fetched by the [first-boot]
// hook: root is the login, so it plants the run key (belt and braces
// with root-ssh-keys, which older answer-file versions may ignore),
// sets the hostname and makes sure sshd is up for wait-ssh.
func renderProxmoxFirstboot(p *Profile, keys []string) string {
	return `#!/bin/sh
mkdir -p /root/.ssh
cat > /root/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /root/.ssh
chmod 600 /root/.ssh/authorized_keys
hostnamectl set-hostname ` + p.Identity.Hostname + ` 2>/dev/null || echo ` + p.Identity.Hostname + ` > /etc/hostname
systemctl enable --now ssh 2>/dev/null || systemctl enable --now sshd 2>/dev/null || true
`
}

// --- NixOS serial-driven nixos-install --------------------------------

// renderNixosConfig writes the target configuration.nix fetched into
// the installed system before nixos-install: declarative user, sudo,
// sshd and qemu-guest in one file — NixOS is the one distro where the
// answer file IS the whole system definition.
func renderNixosConfig(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	locale := p.Locale.Lang
	if locale == "" {
		locale = "en_US.UTF-8"
	}
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "      %q\n", strings.TrimSpace(k))
	}
	return `{ config, pkgs, ... }:
{
  imports = [ ./hardware-configuration.nix ];
  boot.loader.grub.device = "/dev/vda";
  networking.hostName = "` + host + `";
  networking.networkmanager.enable = true;
  time.timeZone = "` + tz + `";
  i18n.defaultLocale = "` + locale + `";
  users.users.` + user + ` = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    hashedPassword = "` + hash + `";
    openssh.authorizedKeys.keys = [
` + sb.String() + `    ];
  };
  security.sudo.wheelNeedsPassword = false;
  services.openssh.enable = true;
  services.qemuGuest.enable = true;
  system.stateVersion = "24.11";
}
`
}

// renderNixosInstall is the script typed-into the live ISO: partition,
// mount, generate the hardware config, pull the declarative
// configuration from the embedded server and nixos-install. The
// trailing poweroff is the completion signal.
func renderNixosInstall(p *Profile, port int) string {
	return `#!/bin/sh
set -e
parted -s /dev/vda mklabel msdos
parted -s /dev/vda mkpart primary ext4 1MiB 100%
mkfs.ext4 -F /dev/vda1
mount /dev/vda1 /mnt
nixos-generate-config --root /mnt
curl -sf -o /mnt/etc/nixos/configuration.nix http://10.0.2.2:` + itoa(port) + `/configuration.nix
nixos-install --no-root-passwd --no-channel-copy
poweroff
`
}

// itoa avoids importing strconv for the two spots that need it.
func itoa(n int) string { return fmt.Sprintf("%d", n) }

// --- OpenBSD install.conf ------------------------------------------

// renderOpenBSDConf writes the response file OpenBSD's autoinstall
// fetches over HTTP. Answers key on the installer's question text —
// the documented autoinstall(8) format. Keys land via the "Public
// ssh key" answers; root password is left locked (bcrypt $2b$ hashes
// aren't what we generate, and sshd only takes keys for root anyway).
func renderOpenBSDConf(p *Profile, keys []string) string {
	user := p.Identity.Username
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "System hostname = %s\n", p.Identity.Hostname)
	sb.WriteString("Which network interface = vio0\n")
	sb.WriteString("IPv4 address for vio0 = dhcp\n")
	fmt.Fprintf(&sb, "Password for root = %s\n", strings.Repeat("*", 13))
	fmt.Fprintf(&sb, "Setup a user = %s\n", user)
	fmt.Fprintf(&sb, "Password for user %s = %s\n", user, strings.Repeat("*", 13))
	for _, k := range keys {
		fmt.Fprintf(&sb, "Public ssh key for user %s = %s\n", user, strings.TrimSpace(k))
	}
	fmt.Fprintf(&sb, "Public ssh key for root = %s\n", strings.TrimSpace(keys[0]))
	fmt.Fprintf(&sb, "What timezone = %s\n", tz)
	sb.WriteString("Which disk is the root disk = sd0\n")
	sb.WriteString("Use (W)hole disk or (E)dit the MBR = whole\n")
	sb.WriteString("Use (A)uto layout, (E)dit auto layout, or create (C)ustom layout = a\n")
	sb.WriteString("Location of sets = cd0\n")
	sb.WriteString("Set name(s) = -game*\n")
	sb.WriteString("Directory does not exist. Continue anyway = yes\n")
	sb.WriteString("Continue without verification = yes\n")
	return sb.String()
}

// --- FreeBSD Live-CD install script ----------------------------------

// renderFreeBSDInstall writes the script run inside the Live CD:
// gpart partition (BIOS/UEFI detected via machdep.bootmethod), the
// ISO's own dist tarballs extracted to /mnt, rc.conf for sshd+dhcp,
// pw for the run user, and pkg-installed sudo so provision works.
func renderFreeBSDInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	return `#!/bin/sh
set -e
DISK=vtbd0
dhclient vtnet0 2>/dev/null || true
gpart destroy -F $DISK 2>/dev/null || true
gpart create -s gpt $DISK
if [ "$(sysctl -n machdep.bootmethod)" = "UEFI" ]; then
  gpart add -t efi -s 260m -l efisys $DISK
  gpart add -t freebsd-ufs -l rootfs $DISK
  newfs_msdos -F 32 /dev/${DISK}p1
  newfs -U /dev/${DISK}p2
  mount /dev/${DISK}p2 /mnt
  mkdir -p /mnt/boot/efi
  mount -t msdosfs /dev/${DISK}p1 /mnt/boot/efi
  mkdir -p /mnt/boot/efi/efi/boot
  cp /boot/loader.efi /mnt/boot/efi/efi/boot/bootx64.efi
  ROOT=/dev/${DISK}p2
  EFIFS="/dev/${DISK}p1 /boot/efi msdosfs rw 2 2"
else
  gpart add -t freebsd-boot -s 512k -l gptboot $DISK
  gpart add -t freebsd-ufs -l rootfs $DISK
  gpart bootcode -b /boot/pmbr -p /boot/gptboot -i 1 $DISK
  newfs -U /dev/${DISK}p2
  mount /dev/${DISK}p2 /mnt
  ROOT=/dev/${DISK}p2
  EFIFS=""
fi
cd /usr/freebsd-dist
for f in kernel.txz base.txz; do tar -xJf "$f" -C /mnt; done
echo "$ROOT / ufs rw 1 1" > /mnt/etc/fstab
[ -n "$EFIFS" ] && echo "$EFIFS" >> /mnt/etc/fstab
echo 'hostname="` + host + `"' >> /mnt/etc/rc.conf
echo 'sshd_enable="YES"' >> /mnt/etc/rc.conf
echo 'ifconfig_DEFAULT="DHCP"' >> /mnt/etc/rc.conf
echo 'qemu_guest_agent_enable="YES"' >> /mnt/etc/rc.conf
ln -sf /usr/share/zoneinfo/` + p.Locale.Timezone + ` /mnt/etc/localtime
pw -R /mnt useradd -n ` + user + ` -m -s /bin/sh -G wheel
echo '` + hash + `' | pw -R /mnt usermod -n ` + user + ` -H 0
mkdir -p /mnt/home/` + user + `/.ssh
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
pw -R /mnt groupmod wheel -m ` + user + `
pw -R /mnt chown -R ` + user + ` /home/` + user + `/.ssh
# sudo is not in base — provision needs it; pkg pulls it into /mnt.
env ASSUME_ALWAYS_YES=yes pkg -r /mnt install -y sudo qemu-guest-agent 2>/dev/null || true
mkdir -p /mnt/usr/local/etc/sudoers.d
echo '` + user + ` ALL=(ALL) NOPASSWD: ALL' > /mnt/usr/local/etc/sudoers.d/` + user + `
chmod 440 /mnt/usr/local/etc/sudoers.d/` + user + `
poweroff
`
}

// --- Void Linux xbps install script -----------------------------------

// renderVoidInstall writes the script run inside the live ISO:
// partition + xbps-install -r /mnt base-system straight off the void
// mirror (musl editions use the musl repo dir), then xchroot for the
// user/grub bits.
func renderVoidInstall(p *Profile, hash string, keys []string, musl bool) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	repo := "https://repo-default.voidlinux.org/current"
	if musl {
		repo += "/musl"
	}
	return `#!/bin/sh
set -e
REPO=` + repo + `
printf 'label: dos\n, , L, *\n' | sfdisk /dev/vda
mkfs.ext4 -F /dev/vda1
mount /dev/vda1 /mnt
mkdir -p /mnt/var/db/xbps/keys
cp -a /var/db/xbps/keys/* /mnt/var/db/xbps/keys/
xbps-install -Sy -R $REPO -r /mnt base-system openssh sudo dhcpcd grub
for d in dev proc sys run; do mount --rbind /$d /mnt/$d; done
cp /etc/resolv.conf /mnt/etc/
chroot /mnt xbps-reconfigure -fa
echo 'hostname=` + host + `' >> /mnt/etc/rc.conf
ln -sf /usr/share/zoneinfo/` + tz + ` /mnt/etc/localtime
chroot /mnt useradd -m -G wheel -s /bin/bash ` + user + `
chroot /mnt usermod -p '` + hash + `' ` + user + `
mkdir -p /mnt/etc/sudoers.d /mnt/home/` + user + `/.ssh
echo '` + user + ` ALL=(ALL) NOPASSWD:ALL' > /mnt/etc/sudoers.d/` + user + `
chmod 440 /mnt/etc/sudoers.d/` + user + `
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
chroot /mnt chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh
for s in sshd dhcpcd; do ln -sf /etc/sv/$s /mnt/etc/runit/runsvdir/default/; done
grub-install --target=i386-pc --boot-directory=/mnt/boot /dev/vda
chroot /mnt grub-mkconfig -o /boot/grub/grub.cfg
poweroff
`
}

// --- Gentoo stage3 install script -------------------------------------

// renderGentooInstall writes the manual-install script: partition,
// stage3 tarball fetch from the distfiles mirror, chroot with binary
// kernel + grub. emerge --getbinpkg keeps it minutes not hours.
func renderGentooInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	locale := p.Locale.Lang
	if locale == "" {
		locale = "en_US.UTF-8"
	}
	return `#!/bin/sh
set -e
STAGE3=https://distfiles.gentoo.org/releases/amd64/autobuilds/current-stage3-amd64-systemd/stage3-amd64-systemd-latest.tar.xz
printf 'label: dos\n, , L, *\n' | sfdisk /dev/vda
mkfs.ext4 -F /dev/vda1
mount /dev/vda1 /mnt
cd /mnt
curl -fLO $STAGE3
tar xpf stage3-*.tar.xz --xattrs-include='*.*' --numeric-owner
rm stage3-*.tar.xz
for d in proc sys dev run; do mount --rbind /$d /mnt/$d; done
cp /etc/resolv.conf /mnt/etc/
cat > /mnt/root/setup.sh <<'CHROOT'
set -e
emerge-webrsync -q || true
echo 'GRUB_PLATFORMS="pc"' >> /etc/portage/make.conf
echo 'USE="getbinpkg"' >> /etc/portage/make.conf
emerge --getbinpkg -q sys-kernel/gentoo-kernel-bin sys-boot/grub net-misc/openssh app-admin/sudo net-misc/dhcpcd app-emulation/qemu-guest-agent
useradd -m -G wheel -s /bin/bash ` + user + `
usermod -p '` + hash + `' ` + user + `
mkdir -p /etc/sudoers.d /home/` + user + `/.ssh
echo '` + user + ` ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/` + user + `
chmod 440 /etc/sudoers.d/` + user + `
cat > /home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /home/` + user + `/.ssh
chmod 600 /home/` + user + `/.ssh/authorized_keys
chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh
grub-install --target=i386-pc /dev/vda
grub-mkconfig -o /boot/grub/grub.cfg
systemctl enable sshd dhcpcd qemu-guest-agent
CHROOT
chroot /mnt /bin/bash /root/setup.sh
echo '` + host + `' > /mnt/etc/hostname
ln -sf ../usr/share/zoneinfo/` + tz + ` /mnt/etc/localtime
echo 'LANG=` + locale + `' > /mnt/etc/env.d/02locale
echo '/dev/vda1 / ext4 defaults 0 1' > /mnt/etc/fstab
poweroff
`
}

// --- Flatcar: ignition + install script -------------------------------

// renderFlatcarIgnition writes the ignition.json flatcar-install
// embeds into the disk image — Container-Linux format: user, keys,
// hostname file.
func renderFlatcarIgnition(p *Profile, hash string, keys []string) string {
	var sb strings.Builder
	for _, k := range keys {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "%q", strings.TrimSpace(k))
	}
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	return `{"ignition":{"version":"3.3.0"},
"passwd":{"users":[
{"name":"` + p.Identity.Username + `","passwordHash":"` + hash + `",
"groups":["sudo","wheel"],"sshAuthorizedKeys":[` + sb.String() + `]}]},
"storage":{"files":[
{"path":"/etc/hostname","contents":{"source":"data:,` + p.Identity.Hostname + `"}},
{"path":"/etc/localtime","contents":{"source":"data:,` + tz + `"}},
{"path":"/etc/sudoers.d/` + p.Identity.Username + `","mode":288,
"contents":{"source":"data:,` + p.Identity.Username + `%20ALL%3D(ALL)%20NOPASSWD%3AALL%0A"}}
]},
"systemd":{"units":[
{"name":"sshd.socket","enabled":true},
{"name":"qemu-guest-agent.service","enabled":true}
]}}`
}

// renderFlatcarInstall is the two-liner typed into the live env:
// fetch ignition, flatcar-install to disk, reboot.
func renderFlatcarInstall(port int) string {
	return `#!/bin/sh
set -e
curl -sf -o /tmp/ign.json http://10.0.2.2:` + itoa(port) + `/ignition.json
sudo flatcar-install -d /dev/vda -i /tmp/ign.json
sudo reboot
`
}

// --- Fedora CoreOS fw_cfg ignition -------------------------------------

// renderCoreOSIgnition is the Butane-free Ignition for the qcow2's
// first boot — same schema family as flatcar (Ignition v3), writing
// user/keys/hostname so wait-ssh and provision work like nocloud.
func renderCoreOSIgnition(p *Profile, hash string, keys []string) string {
	var sb strings.Builder
	for _, k := range keys {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "%q", strings.TrimSpace(k))
	}
	return `{"ignition":{"version":"3.4.0"},
"passwd":{"users":[
{"name":"` + p.Identity.Username + `","passwordHash":"` + hash + `",
"groups":["wheel"],"sshAuthorizedKeys":[` + sb.String() + `],
"sshAuthorizedKeysManaged":false}]},
"storage":{"files":[
{"path":"/etc/hostname","contents":{"source":"data:,` + p.Identity.Hostname + `"}},
{"path":"/etc/sudoers.d/` + p.Identity.Username + `","mode":288,
"contents":{"source":"data:,` + p.Identity.Username + `%20ALL%3D(ALL)%20NOPASSWD%3AALL%0A"}}
]},
"systemd":{"units":[
{"name":"qemu-guest-agent.service","enabled":true}
]}}`
}

// --- Ubiquity preseed (mint, zorin, xubuntu, mate) -------------------
//
// renderUbiquityPreseed answers ubiquity's debconf questions — a
// d-i-shaped preseed but without mirror keys (the squashfs is the
// payload) and with the ubiquity/* questions that keep the wizard
// from stopping. `ubiquity/poweroff true` ends in shutdown.
func renderUbiquityPreseed(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	lang := p.Locale.Lang
	if lang == "" {
		lang = "en_US.UTF-8"
	}
	kbd := p.Locale.Keyboard
	if kbd == "" {
		kbd = "us"
	}
	var k strings.Builder
	for _, key := range keys {
		k.WriteString(strings.TrimSpace(key) + " ")
	}
	return `d-i debian-installer/locale string ` + lang + `
d-i keyboard-configuration/xkb-keymap select ` + kbd + `
ubiquity language/language string ` + strings.SplitN(lang, ".", 2)[0] + `
ubiquity localechooser/shortlist select ` + lang + `
d-i netcfg/get_hostname string ` + host + `
d-i netcfg/get_domain string local
ubiquity timezone select ` + tz + `
d-i time/zone string ` + tz + `
d-i passwd/root-login boolean false
d-i passwd/user-fullname string ` + user + `
d-i passwd/username string ` + user + `
d-i passwd/user-password-crypted password ` + hash + `
d-i user-setup/allow-password-weak boolean true
ubiquity user-setup/encrypt-home boolean false
d-i partman-auto/method string regular
d-i partman-auto/choose_recipe select atomic
d-i partman/confirm_write_new_label boolean true
d-i partman/choose_partition select finish
d-i partman/confirm boolean true
d-i partman/confirm_nooverwrite boolean true
d-i finish-install/reboot_in_progress note
ubiquity ubiquity/poweroff boolean true
ubiquity ubiquity/reboot boolean false
ubiquity ubiquity/summary string {}
ubiquity ubiquity/success_command string in-target sh -c 'apt-get update -qq && apt-get install -y openssh-server || true; echo "` + user + ` ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/` + user + ` && chmod 440 /etc/sudoers.d/` + user + ` && mkdir -p /home/` + user + `/.ssh && echo "` + strings.TrimSpace(k.String()) + `" > /home/` + user + `/.ssh/authorized_keys && chmod 700 /home/` + user + `/.ssh && chmod 600 /home/` + user + `/.ssh/authorized_keys && chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh'
`
}

// --- Slackware install script -----------------------------------------

// renderSlackwareInstall writes the typed-in script for the install
// media's root shell: partition, installpkg every series from the CD
// (full install — Slackware's own recommendation), then lilo/elilo,
// fstab, HOSTNAME, DHCP, sshd and the run user — none of the dialog
// tools (setup, liloconfig, netconfig) are needed when the config
// files are written directly.
func renderSlackwareInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	return `#!/bin/sh
set -e
mkdir -p /cdrom /mnt
mount /dev/sr0 /cdrom
SERIES=slackware64
[ -d /cdrom/slackware ] && SERIES=slackware
[ -d /cdrom/slackware64 ] && SERIES=slackware64
printf 'label: dos\n, , L, *\n' | sfdisk /dev/vda
mkfs.ext4 -F /dev/vda1
mount /dev/vda1 /mnt
for d in /cdrom/$SERIES/*/; do
  installpkg --root /mnt --terse "$d"*.t?z
done
mkdir -p /mnt/etc
echo '/dev/vda1 / ext4 defaults 1 1' > /mnt/etc/fstab
echo '` + host + `' > /mnt/etc/HOSTNAME
# DHCP on the virtio nic via rc.inet1.
sed -i 's/^USE_DHCP\[0\]=""/USE_DHCP[0]="yes"/' /mnt/etc/rc.d/rc.inet1.conf
chmod +x /mnt/etc/rc.d/rc.sshd
mkdir -p /mnt/etc/sudoers.d /mnt/home/` + user + `/.ssh
useradd --root /mnt -m -g users -G wheel -s /bin/bash ` + user + ` || \
  chroot /mnt useradd -m -g users -G wheel -s /bin/bash ` + user + `
echo '` + user + `:` + hash + `' | chpasswd --root /mnt -e || \
  chroot /mnt sh -c 'echo "` + user + `:` + hash + `" | chpasswd -e'
echo '` + user + ` ALL=(ALL) NOPASSWD: ALL' > /mnt/etc/sudoers.d/` + user + `
chmod 440 /mnt/etc/sudoers.d/` + user + `
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
chroot /mnt chown -R ` + user + `:users /home/` + user + `/.ssh
if [ -d /sys/firmware/efi ]; then
  mkdir -p /mnt/boot/efi/EFI/Slackware
  cp /mnt/boot/vmlinuz-huge-* /mnt/boot/efi/EFI/Slackware/vmlinuz 2>/dev/null || \
    cp /mnt/boot/vmlinuz /mnt/boot/efi/EFI/Slackware/
  cat > /mnt/boot/efi/EFI/Slackware/elilo.conf <<'ELILO'
image=vmlinuz
label=slackware
root=/dev/vda1
read-only
ELILO
else
  KIMG=$(basename /mnt/boot/vmlinuz-huge-* 2>/dev/null | head -1)
  [ -z "$KIMG" ] && KIMG=vmlinuz
  cat > /mnt/etc/lilo.conf <<LILO
boot = /dev/vda
image = /boot/$KIMG
root = /dev/vda1
label = slackware
read-only
LILO
  chroot /mnt /sbin/lilo
fi
poweroff
`
}

// --- Artix basestrap script -------------------------------------------

// renderArtixInstall mirrors the arch script with artix's basestrap:
// openrc stack (ISOs ship per-init editions but the base repos carry
// all init pkgs), fstabgen, artix-chroot for user/grub/rc-update.
func renderArtixInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	locale := p.Locale.Lang
	if locale == "" {
		locale = "en_US.UTF-8"
	}
	return `#!/bin/sh
set -e
printf 'label: dos\n, , L, *\n' | sfdisk /dev/vda
mkfs.ext4 -F /dev/vda1
mount /dev/vda1 /mnt
pacman-key --init 2>/dev/null || true
pacman-key --populate artix 2>/dev/null || true
basestrap /mnt base base-devel openrc elogind-openrc linux linux-firmware \
  openssh openssh-openrc dhcpcd dhcpcd-openrc sudo grub os-prober
fstabgen -U /mnt >> /mnt/etc/fstab
echo '` + host + `' > /mnt/etc/hostname
sed -i 's/^#\(` + locale + `\)/\1/' /mnt/etc/locale.gen
artix-chroot /mnt locale-gen
echo 'LANG=` + locale + `' > /mnt/etc/locale.conf
artix-chroot /mnt useradd -m -G wheel -s /bin/bash ` + user + `
artix-chroot /mnt usermod -p '` + hash + `' ` + user + `
mkdir -p /mnt/etc/sudoers.d /mnt/home/` + user + `/.ssh
echo '` + user + ` ALL=(ALL) NOPASSWD:ALL' > /mnt/etc/sudoers.d/` + user + `
chmod 440 /mnt/etc/sudoers.d/` + user + `
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
artix-chroot /mnt chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh
for s in sshd dhcpcd; do artix-chroot /mnt rc-update add $s default; done
if [ -d /sys/firmware/efi ]; then
  artix-chroot /mnt grub-install --target=x86_64-efi --efi-directory=/boot --bootloader-id=artix --removable
else
  artix-chroot /mnt grub-install --target=i386-pc /dev/vda
fi
artix-chroot /mnt grub-mkconfig -o /boot/grub/grub.cfg
poweroff
`
}

// --- Calculate cl-install script ---------------------------------------

// renderCalculateInstall drives Calculate's scriptable installer:
// `cl-install -d` does a whole-disk install from the live image, then
// the script chroots for user/sshd/network defaults.
func renderCalculateInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	return `#!/bin/sh
set -e
cl-install -d /dev/vda -u root || cl-install --disk /dev/vda
mount /dev/vda1 /mnt 2>/dev/null || mount /dev/vda3 /mnt
for d in proc sys dev run; do mount --rbind /$d /mnt/$d; done
cp /etc/resolv.conf /mnt/etc/
chroot /mnt useradd -m -G wheel -s /bin/bash ` + user + ` || true
chroot /mnt usermod -p '` + hash + `' ` + user + `
mkdir -p /mnt/etc/sudoers.d /mnt/home/` + user + `/.ssh
echo '` + user + ` ALL=(ALL) NOPASSWD:ALL' > /mnt/etc/sudoers.d/` + user + `
chmod 440 /mnt/etc/sudoers.d/` + user + `
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
chroot /mnt chown -R ` + user + `:` + user + ` /home/` + user + `/.ssh
echo 'hostname="` + host + `"' >> /mnt/etc/conf.d/hostname 2>/dev/null || \
  echo '` + host + `' > /mnt/etc/hostname
chroot /mnt rc-update add sshd default 2>/dev/null || true
chroot /mnt rc-update add dhcpcd default 2>/dev/null || true
poweroff
`
}

// --- Ubuntu desktop (ubuntu-desktop-bootstrap/subiquity) ----------

// renderDesktopAutoinstall is the desktop installer's schema — a
// narrower autoinstall subset than the server's (no late-commands,
// no shutdown directive). Install completion is detected on the
// serial console instead (curtin's "Installation finished").
func renderDesktopAutoinstall(p *Profile, hash, keysYAML string) string {
	var sb strings.Builder
	sb.WriteString("#cloud-config\nautoinstall:\n  version: 1\n  interactive-sections: []\n")
	sb.WriteString("  identity:\n")
	fmt.Fprintf(&sb, "    hostname: %s\n    username: %s\n    password: %s\n",
		p.Identity.Hostname, p.Identity.Username, yamlQ(hash))
	if p.Locale.Lang != "" {
		fmt.Fprintf(&sb, "  locale: %s\n", p.Locale.Lang)
	}
	if p.Locale.Keyboard != "" {
		fmt.Fprintf(&sb, "  keyboard:\n    layout: %s\n", p.Locale.Keyboard)
	}
	if p.Locale.Timezone != "" {
		fmt.Fprintf(&sb, "  timezone: %s\n", p.Locale.Timezone)
	}
	sb.WriteString("  ssh:\n    install-server: true\n")
	fmt.Fprintf(&sb, "    allow-pw: %v\n", p.Identity.AllowPwAuth || p.Identity.Password != "")
	sb.WriteString("    authorized-keys:\n")
	sb.WriteString(keysYAML)
	sb.WriteString("  storage:\n    layout:\n      name: direct\n")
	sb.WriteString("  source:\n    id: ubuntu-desktop\n")
	return sb.String()
}

// --- Guix -------------------------------------------------------------
//
// renderGuixInstall runs inside the live image (before cow-store): it
// partitions, mounts, pulls the rendered config.scm + authorized keys
// over slirp, then hands the target to guix system init. The efi/bios
// split picks which rendered config it asks for.
func renderGuixInstall(port int) string {
	return `#!/bin/sh
set -e
dhclient 2>/dev/null || true
if [ -d /sys/firmware/efi ]; then
  parted -s /dev/vda mklabel gpt
  parted -s /dev/vda mkpart ESP fat32 1MiB 513MiB
  parted -s /dev/vda set 1 esp on
  parted -s /dev/vda mkpart root ext4 513MiB 100%
  mkfs.fat -F32 /dev/vda1
  mkfs.ext4 -F -L guix-root /dev/vda2
  mount /dev/vda2 /mnt
  mkdir -p /mnt/boot/efi
  mount /dev/vda1 /mnt/boot/efi
  CFG=config-efi.scm
else
  parted -s /dev/vda mklabel msdos
  parted -s /dev/vda mkpart primary ext4 1MiB 100%
  parted -s /dev/vda set 1 boot on
  mkfs.ext4 -F -L guix-root /dev/vda1
  mount /dev/vda1 /mnt
  CFG=config-bios.scm
fi
mkdir -p /mnt/etc /mnt/root
curl -sf -o /mnt/etc/config.scm http://10.0.2.2:` + itoa(port) + `/$CFG
curl -sf -o /mnt/root/keys.pub http://10.0.2.2:` + itoa(port) + `/keys.pub
herd start cow-store /mnt
guix system init /mnt/etc/config.scm /mnt
poweroff
`
}

// renderGuixConfig emits the declarative operating-system form the live
// image installs verbatim — the bootloader variant switches on firmware.
func renderGuixConfig(p *Profile, hash string, efi bool) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	tz := p.Locale.Timezone
	if tz == "" {
		tz = "UTC"
	}
	lang := p.Locale.Lang
	if lang == "" {
		lang = "en_US.utf8"
	}
	bl := `(bootloader grub-bootloader)
                (targets '("/dev/vda"))`
	fses := `(file-system (device (file-system-label "guix-root"))
                         (mount-point "/") (type "ext4"))`
	if efi {
		bl = `(bootloader grub-efi-bootloader)
                (targets '("/boot/efi"))`
		fses += `
                       (file-system (device "/dev/vda1")
                         (mount-point "/boot/efi") (type "vfat"))`
	}
	return `(use-modules (gnu) (gnu system nss))
(use-service-modules networking ssh)
(operating-system
  (host-name "` + host + `")
  (timezone "` + tz + `")
  (locale "` + lang + `")
  (bootloader (bootloader-configuration
                ` + bl + `))
  (file-systems (cons* ` + fses + `
                       %base-file-systems))
  (users (cons* (user-account (name "` + user + `") (group "users")
                  (supplementary-groups '("wheel" "netdev"))
                  (password "` + hash + `"))
                %base-user-accounts))
  (services (cons* (service openssh-service-type
                     (openssh-configuration
                       (authorized-keys
                         ` + "`" + `(("` + user + `" ,(local-file "/root/keys.pub"))))))
                   (service dhcp-client-service-type)
                   %base-services))))
`
}

// --- NetBSD -----------------------------------------------------------
//
// renderNetbsdInstall runs in the sysinst utility-menu shell: GPT
// wedges keep partitioning deterministic, sets untar straight off the
// mounted install CD (no network needed for payload), bootblocks and
// rc.conf/fstab/user finish the disk. The NAME=<wedge-label> fstab
// syntax survives device renumbering.
func renderNetbsdInstall(p *Profile, hash string, keys []string) string {
	user := p.Identity.Username
	host := p.Identity.Hostname
	return `#!/bin/sh
set -e
mkdir -p /cdrom /mnt
mount_cd9660 /dev/cd0a /cdrom 2>/dev/null || mount_cd9660 /dev/cd0d /cdrom
SETS=/cdrom/amd64/binary/sets
[ -d "$SETS" ] || SETS=$(echo /cdrom/*/binary/sets | cut -d' ' -f1)
gpt destroy wd0 2>/dev/null || true
gpt create -f wd0
if [ -d /sys/firmware/efi ]; then
  gpt add -a 1m -s 128m -t efi -l ESP wd0
  gpt add -a 1m -t ffs -l ROOT wd0
else
  gpt add -a 1m -s 32m -t bios -l BIOSBOOT wd0
  gpt add -a 1m -t ffs -l ROOT wd0
fi
RDK=$(dkctl wd0 listwedges | awk -F: '/ROOT/{print $1}' | head -1)
EFIDK=$(dkctl wd0 listwedges | awk -F: '/ESP/{print $1}' | head -1)
newfs -O2 /dev/r$(basename $RDK)
if [ -n "$EFIDK" ]; then
  newfs_msdos -F 32 /dev/r$(basename $EFIDK)
fi
mount $RDK /mnt
for f in $SETS/*.tar.xz $SETS/*.tgz; do
  [ -f "$f" ] || continue
  xz -dc "$f" 2>/dev/null | tar -xpf - -C /mnt || \
    unxz -c "$f" 2>/dev/null | tar -xpf - -C /mnt || \
    tar -xpf "$f" -C /mnt || \
    gunzip -c "$f" | tar -xpf - -C /mnt
done
mkdir -p /mnt/etc
{
  echo 'hostname=` + host + `'
  echo 'dhcpcd=YES'
  echo 'sshd=YES'
} >> /mnt/etc/rc.conf
{
  echo 'NAME=ROOT / ffs rw 1 1'
  if [ -n "$EFIDK" ]; then echo 'NAME=ESP /efi msdos rw 1 2'; fi
  echo 'kernfs /kern kernfs rw'
  echo 'ptyfs /dev/pts ptyfs rw,noauto'
  echo 'procfs /proc procfs rw'
  echo 'tmpfs /var/shm tmpfs rw,-s=1g'
} > /mnt/etc/fstab
if [ -n "$EFIDK" ]; then
  mkdir -p /mnt/efi
  mount -t msdos $EFIDK /mnt/efi
  mkdir -p /mnt/efi/EFI/BOOT
  cp /mnt/usr/mdec/bootx64.efi /mnt/efi/EFI/BOOT/BOOTX64.EFI
else
  installboot -o timeout=5 /dev/r$(basename $RDK) /mnt/usr/mdec/bootxx_ffsv2
  gpt biosboot -A -c /mnt/usr/mdec/gptmbr.bin -L BIOSBOOT wd0 2>/dev/null || \
    gpt biosboot -c /mnt/usr/mdec/gptmbr.bin -i 1 wd0 || true
fi
chroot /mnt sh -c 'useradd -m -G wheel -s /bin/sh ` + user + `; \
  usermod -p "` + hash + `" ` + user + `'
mkdir -p /mnt/home/` + user + `/.ssh
cat > /mnt/home/` + user + `/.ssh/authorized_keys <<'KEYS'
` + joinKeys(keys) + `KEYS
chmod 700 /mnt/home/` + user + `/.ssh
chmod 600 /mnt/home/` + user + `/.ssh/authorized_keys
chroot /mnt chown -R ` + user + `:users /home/` + user + `/.ssh
sync
poweroff 2>/dev/null || reboot 2>/dev/null || halt
`
}

// renderOpenwrtConfig is the first-boot script typed into the serial
// root shell: hostname, root credential, dropbear key, lan→dhcp so
// slirp can reach sshd on the configured image.
func renderOpenwrtConfig(p *Profile, hash string, keys []string) string {
	return "#!/bin/ash\n" +
		"set -e\n" +
		"uci set system.@system[0].hostname='" + p.Identity.Hostname + "'\n" +
		"uci commit system\n" +
		"# root credential — sed the hash into /etc/shadow (no interactive passwd)\n" +
		"sed -i 's|^root:[^:]*:|root:" + hash + ":|' /etc/shadow\n" +
		"mkdir -p /etc/dropbear\n" +
		"cat > /etc/dropbear/authorized_keys <<'KEYS'\n" +
		joinKeys(keys) +
		"KEYS\n" +
		"chmod 700 /etc/dropbear\n" +
		"chmod 600 /etc/dropbear/authorized_keys\n" +
		"/etc/init.d/dropbear enable\n" +
		"# stock lan is static 192.168.1.1 — unreachable behind slirp; dhcp gets 10.0.2.15\n" +
		"uci set network.lan.proto='dhcp'\n" +
		"uci -q delete network.lan.ipaddr\n" +
		"uci -q delete network.lan.netmask\n" +
		"uci commit network\n" +
		"sync\n" +
		"poweroff\n"
}

// renderRouterosConfig is the .rsc /import'd on the serial CLI: ssh key
// for admin, dhcp client on ether1, ssh service up, identity set.
// dhcp add is on-error-guarded — CHR images already ship a client on
// some releases.
func renderRouterosConfig(p *Profile) string {
	return "/user ssh-keys import public-key-file=qp.pub user=admin\n" +
		"/ip service set ssh disabled=no\n" +
		":do { /ip dhcp-client add disabled=no interface=ether1 } on-error={}\n" +
		"/system identity set name=\"" + p.Identity.Hostname + "\"\n"
}

// --- Alpine setup-alpine (apkovl overlay) ------------------------------
//
// The alpine ISO has no answer-file mechanism of its own: the
// canonical unattended path is an apkovl — a gzipped tar overlaid on
// the live rootfs by the initramfs when `apkovl=` is on the kernel
// cmdline. The overlay below carries a setup-alpine answer file and a
// local.d script enabled in the default runlevel that runs
// `setup-alpine -ef` and powers off (the runner's completion signal).

// renderAlpineAnswers emits the shell-sourced answer file consumed by
// `setup-alpine -ef`. DISKOPTS sys-installs onto the virtio disk; the
// erase confirmation is bypassed by ERASE_DISKS, which setup-disk
// reads from its environment — the runner script exports it because
// variables sourced from an answer file are not exported.
//
// APKREPOSOPTS carries literal repositories: setup-apkrepos writes
// positional arguments verbatim, which avoids the mirrors.txt fetch
// (guest HTTPS through slirp is unreliable — apk still reaches the
// CDN over plain HTTP).
func renderAlpineAnswers(p *Profile, pubkeys, release string) string {
	return "# generated by quickprovision — alpine unattended install\n" +
		"KEYMAPOPTS=none\n" +
		"HOSTNAMEOPTS=" + p.Identity.Hostname + "\n" +
		"DEVDOPTS=mdev\n" +
		"INTERFACESOPTS=\"auto lo\n" +
		"iface lo inet loopback\n" +
		"\n" +
		"auto eth0\n" +
		"iface eth0 inet dhcp\"\n" +
		"DNSOPTS=none\n" +
		"TIMEZONEOPTS=UTC\n" +
		"PROXYOPTS=none\n" +
		"APKREPOSOPTS=\"" + alpineRepos(release) + "\"\n" +
		"SSHDOPTS=openssh\n" +
		"ROOTSSHKEY=\"" + pubkeys + "\"\n" +
		"USEROPTS=\"-a -u -g audio,input,video,netdev " + p.Identity.Username + "\"\n" +
		"USERSSHKEY=\"" + pubkeys + "\"\n" +
		"NTPOPTS=none\n" +
		"DISKOPTS=\"-m sys /dev/vda\"\n" +
		"LBUOPTS=none\n" +
		"APKCACHEOPTS=none\n"
}

// alpineRepos returns the repository list written into the live and
// installed /etc/apk/repositories: the mounted ISO's local apks first
// (always correct for the base set), then the CDN over plain HTTP —
// setup-disk pulls the bootloader (grub-efi under OVMF) from there.
func alpineRepos(release string) string {
	if release == "" {
		release = "latest-stable"
	}
	return "/media/cdrom/apks " +
		"http://dl-cdn.alpinelinux.org/alpine/" + release + "/main " +
		"http://dl-cdn.alpinelinux.org/alpine/" + release + "/community"
}

// alpineAutoStart is the local.d hook the overlay enables: stdin is
// closed so no answer can block, output is mirrored to the console
// (openrc swallows service output into its own log), and poweroff
// always runs so a failed install still terminates the VM instead of
// sitting at a prompt. MIRRORS short-circuits setup-apkrepos'
// unconditional mirrors.txt fetch — repositories are positional.
const alpineAutoStart = `#!/bin/sh
exec 0<&-
exec >/dev/console 2>&1
export ERASE_DISKS=/dev/vda
export MIRRORS=x
setup-alpine -ef /etc/answers
rc=$?
echo "quickprovision: setup-alpine rc=$rc"
poweroff
`

// writeAlpineApkovl builds setup.apkovl — the overlay injected into
// the initramfs and referenced via `apkovl=<path>` — enabling
// networking+sshd+local in the live env and dropping the answers,
// apk repositories and driver script into place.
func writeAlpineApkovl(path string, p *Profile, pubkeys, release string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	addDir := func(name string) error {
		return tw.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeDir, Mode: 0o755})
	}
	addFile := func(name, body string, mode int64) error {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeReg,
			Mode: mode, Size: int64(len(body))}); err != nil {
			return err
		}
		_, err := tw.Write([]byte(body))
		return err
	}
	addLink := func(name, target string) error {
		return tw.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeSymlink, Linkname: target})
	}

	for _, d := range []string{
		"etc", "etc/apk", "etc/network", "etc/runlevels",
		"etc/runlevels/boot", "etc/runlevels/default", "etc/local.d",
		"root", "root/.ssh"} {
		if err := addDir(d); err != nil {
			return err
		}
	}
	// `.default_boot_services` plus the runlevel links mirror the
	// upstream headless.apkovl — networking must be live before
	// local.d runs so setup-alpine can reach the mirror and the
	// embedded HTTP server.
	if err := addFile("etc/.default_boot_services", "", 0o644); err != nil {
		return err
	}
	if err := addFile("etc/network/interfaces",
		"auto lo\niface lo inet loopback\n\n"+
			"auto eth0\niface eth0 inet dhcp\n", 0o644); err != nil {
		return err
	}
	if err := addFile("etc/apk/repositories",
		strings.ReplaceAll(alpineRepos(release), " ", "\n")+"\n",
		0o644); err != nil {
		return err
	}
	for _, link := range [][2]string{
		{"etc/runlevels/boot/networking", "/etc/init.d/networking"},
		{"etc/runlevels/default/sshd", "/etc/init.d/sshd"},
		{"etc/runlevels/default/local", "/etc/init.d/local"},
	} {
		if err := addLink(link[0], link[1]); err != nil {
			return err
		}
	}
	if err := addFile("etc/answers",
		renderAlpineAnswers(p, pubkeys, release), 0o644); err != nil {
		return err
	}
	if err := addFile("etc/local.d/auto-provision.start",
		alpineAutoStart, 0o755); err != nil {
		return err
	}
	if err := addFile("root/.ssh/authorized_keys", pubkeys, 0o600); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// apkovl must live inside artDir for the seed HTTP server — the
	// caller writes it there; keep the mode readable.
	return os.Chmod(filepath.Clean(path), 0o644)
}
