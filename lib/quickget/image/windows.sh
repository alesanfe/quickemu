# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Image: Windows unattended-install answer files.


# True when a filename is a retail Windows 10/11 ISO that the unattended
# generator can drive (Win10_22H2_..._x64.iso style names).
function is_unattended_windows_iso() {
    grep -E -q 'Win(10|11)_([0-9]{2}H(1|2)|[0-9]{4})_[^.]*?(x64|x32)(v[0-9])?.iso' <<< "${1}"
}


function unattended_windows() {
    mkdir -p "${1}/unattended" 2>/dev/null

    # VirtIO driver directory matching the Windows release being installed.
    # qxldod has no w11 build on virtio-win, so it is skipped for Windows 11.
    local DRIVER_DIR="${2:-w10}"
    local QXLDOD_PATH=""
    if [ "${DRIVER_DIR}" == "w10" ]; then
        QXLDOD_PATH='        <PathAndCredentials wcm:action="add" wcm:keyValue="6">
          <Path>E:\qxldod\w10\amd64</Path>
        </PathAndCredentials>'
    fi

    # Resolve the Windows locale for the requested language (default en-US).
    local CULTURE=""
    local LANGID=""
    case "${I18N}" in
        "Arabic") CULTURE="ar-SA"; LANGID="0401";;
        "Brazilian Portuguese") CULTURE="pt-BR"; LANGID="0416";;
        "Bulgarian") CULTURE="bg-BG"; LANGID="0402";;
        "Chinese (Simplified)") CULTURE="zh-CN"; LANGID="0804";;
        "Chinese (Traditional)") CULTURE="zh-TW"; LANGID="0404";;
        "Croatian") CULTURE="hr-HR"; LANGID="041a";;
        "Czech") CULTURE="cs-CZ"; LANGID="0405";;
        "Danish") CULTURE="da-DK"; LANGID="0406";;
        "Dutch") CULTURE="nl-NL"; LANGID="0413";;
        "English International") CULTURE="en-GB"; LANGID="0809";;
        "Estonian") CULTURE="et-EE"; LANGID="0425";;
        "Finnish") CULTURE="fi-FI"; LANGID="040b";;
        "French") CULTURE="fr-FR"; LANGID="040c";;
        "French Canadian") CULTURE="fr-CA"; LANGID="0c0c";;
        "German") CULTURE="de-DE"; LANGID="0407";;
        "Greek") CULTURE="el-GR"; LANGID="0408";;
        "Hebrew") CULTURE="he-IL"; LANGID="040d";;
        "Hungarian") CULTURE="hu-HU"; LANGID="040e";;
        "Italian") CULTURE="it-IT"; LANGID="0410";;
        "Japanese") CULTURE="ja-JP"; LANGID="0411";;
        "Korean") CULTURE="ko-KR"; LANGID="0412";;
        "Latvian") CULTURE="lv-LV"; LANGID="0426";;
        "Lithuanian") CULTURE="lt-LT"; LANGID="0427";;
        "Norwegian") CULTURE="nb-NO"; LANGID="0414";;
        "Polish") CULTURE="pl-PL"; LANGID="0415";;
        "Portuguese") CULTURE="pt-PT"; LANGID="0816";;
        "Romanian") CULTURE="ro-RO"; LANGID="0418";;
        "Russian") CULTURE="ru-RU"; LANGID="0419";;
        "Serbian Latin") CULTURE="sr-Latn-RS"; LANGID="081a";;
        "Slovak") CULTURE="sk-SK"; LANGID="041b";;
        "Slovenian") CULTURE="sl-SI"; LANGID="0424";;
        "Spanish") CULTURE="es-ES"; LANGID="0c0a";;
        "Spanish (Mexico)") CULTURE="es-MX"; LANGID="080a";;
        "Swedish") CULTURE="sv-SE"; LANGID="041d";;
        "Thai") CULTURE="th-TH"; LANGID="041e";;
        "Turkish") CULTURE="tr-TR"; LANGID="041f";;
        "Ukrainian") CULTURE="uk-UA"; LANGID="0422";;
        *) CULTURE="en-US"; LANGID="0409";;
    esac

    cat << EOF > "${1}/unattended/autounattend.xml"
<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend"
  xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <!--
       For documentation on components:
       https://docs.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/
  -->
  <settings pass="offlineServicing">
    <component name="Microsoft-Windows-LUA-Settings" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <EnableLUA>false</EnableLUA>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <ComputerName>*</ComputerName>
    </component>
  </settings>

  <settings pass="generalize">
    <component name="Microsoft-Windows-PnPSysprep" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <PersistAllDeviceInstalls>true</PersistAllDeviceInstalls>
    </component>
    <component name="Microsoft-Windows-Security-SPP" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <SkipRearm>1</SkipRearm>
    </component>
  </settings>

  <settings pass="specialize">
    <component name="Microsoft-Windows-Security-SPP-UX" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <SkipAutoActivation>true</SkipAutoActivation>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <ComputerName>*</ComputerName>
      <OEMInformation>
        <Manufacturer>Quickemu Project</Manufacturer>
        <Model>Quickemu</Model>
        <SupportHours>24/7</SupportHours>
        <SupportPhone></SupportPhone>
        <SupportProvider>Quickemu Project</SupportProvider>
        <SupportURL>https://github.com/quickemu-project/quickemu/issues</SupportURL>
      </OEMInformation>
      <OEMName>Quickemu Project</OEMName>
      <ProductKey>W269N-WFGWX-YVC9B-4J6C9-T83GX</ProductKey>
    </component>
    <component name="Microsoft-Windows-SQMApi" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <CEIPEnabled>0</CEIPEnabled>
    </component>
  </settings>

  <settings pass="windowsPE">
    <component name="Microsoft-Windows-International-Core-WinPE" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <SetupUILanguage>
        <UILanguage>${CULTURE}</UILanguage>
      </SetupUILanguage>
      <InputLocale>${LANGID}:0000${LANGID}</InputLocale>
      <SystemLocale>${CULTURE}</SystemLocale>
      <UILanguage>${CULTURE}</UILanguage>
      <UserLocale>${CULTURE}</UserLocale>
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
            <!-- Windows RE Tools partition -->
            <CreatePartition wcm:action="add">
              <Order>1</Order>
              <Type>Primary</Type>
              <Size>256</Size>
            </CreatePartition>
            <!-- System partition (ESP) -->
            <CreatePartition wcm:action="add">
              <Order>2</Order>
              <Type>EFI</Type>
              <Size>128</Size>
            </CreatePartition>
            <!-- Microsoft reserved partition (MSR) -->
            <CreatePartition wcm:action="add">
              <Order>3</Order>
              <Type>MSR</Type>
              <Size>128</Size>
            </CreatePartition>
            <!-- Windows partition -->
            <CreatePartition wcm:action="add">
              <Order>4</Order>
              <Type>Primary</Type>
              <Extend>true</Extend>
            </CreatePartition>
          </CreatePartitions>
          <ModifyPartitions>
            <!-- Windows RE Tools partition -->
            <ModifyPartition wcm:action="add">
              <Order>1</Order>
              <PartitionID>1</PartitionID>
              <Label>WINRE</Label>
              <Format>NTFS</Format>
              <TypeID>DE94BBA4-06D1-4D40-A16A-BFD50179D6AC</TypeID>
            </ModifyPartition>
            <!-- System partition (ESP) -->
            <ModifyPartition wcm:action="add">
              <Order>2</Order>
              <PartitionID>2</PartitionID>
              <Label>System</Label>
              <Format>FAT32</Format>
            </ModifyPartition>
            <!-- MSR partition does not need to be modified -->
            <ModifyPartition wcm:action="add">
              <Order>3</Order>
              <PartitionID>3</PartitionID>
            </ModifyPartition>
            <!-- Windows partition -->
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
        <FullName>Quickemu</FullName>
        <Organization>Quickemu Project</Organization>
        <!-- https://docs.microsoft.com/en-us/windows-server/get-started/kms-client-activation-keys -->
        <ProductKey>
          <Key>W269N-WFGWX-YVC9B-4J6C9-T83GX</Key>
          <WillShowUI>Never</WillShowUI>
        </ProductKey>
      </UserData>
    </component>

    <component name="Microsoft-Windows-PnpCustomizationsWinPE" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" processorArchitecture="amd64" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <!--
           This makes the VirtIO drivers available to Windows, assuming that
           the VirtIO driver disk is available as drive E:
           https://github.com/virtio-win/virtio-win-pkg-scripts/blob/master/README.md
      -->
      <DriverPaths>
        <PathAndCredentials wcm:action="add" wcm:keyValue="1">
          <Path>E:\qemufwcfg\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="2">
          <Path>E:\vioinput\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="3">
          <Path>E:\vioscsi\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="4">
          <Path>E:\viostor\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="5">
          <Path>E:\vioserial\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
${QXLDOD_PATH}
        <PathAndCredentials wcm:action="add" wcm:keyValue="7">
          <Path>E:\viorng\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="8">
          <Path>E:\NetKVM\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="9">
          <Path>E:\viofs\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="10">
          <Path>E:\Balloon\\${DRIVER_DIR}\amd64</Path>
        </PathAndCredentials>
      </DriverPaths>
    </component>
  </settings>

  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <AutoLogon>
        <Password>
          <Value>quickemu</Value>
          <PlainText>true</PlainText>
        </Password>
        <Enabled>true</Enabled>
        <Username>Quickemu</Username>
      </AutoLogon>
      <DisableAutoDaylightTimeSet>false</DisableAutoDaylightTimeSet>
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
              <Value>quickemu</Value>
              <PlainText>true</PlainText>
            </Password>
            <Description>Quickemu</Description>
            <DisplayName>Quickemu</DisplayName>
            <Group>Administrators</Group>
            <Name>Quickemu</Name>
          </LocalAccount>
        </LocalAccounts>
      </UserAccounts>
      <RegisteredOrganization>Quickemu Project</RegisteredOrganization>
      <RegisteredOwner>Quickemu</RegisteredOwner>
      <FirstLogonCommands>
        <SynchronousCommand wcm:action="add">
          <CommandLine>msiexec /i E:\guest-agent\qemu-ga-x86_64.msi /quiet /passive /qn</CommandLine>
          <Description>Install Virtio Guest Agent</Description>
          <Order>1</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>msiexec /i F:\spice-webdavd-x64-latest.msi /quiet /passive /qn</CommandLine>
          <Description>Install spice-webdavd file sharing agent</Description>
          <Order>2</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>msiexec /i F:\spice-vdagent-x64-0.10.0.msi /quiet /passive /qn</CommandLine>
          <Description>Install spice-vdagent SPICE agent</Description>
          <Order>3</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>Cmd /c POWERCFG -H OFF</CommandLine>
          <Description>Disable Hibernation</Description>
          <Order>4</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>pnputil /add-driver E:\viogpudo\\${DRIVER_DIR}\amd64\viogpudo.inf /install</CommandLine>
          <Description>Install viogpudo driver</Description>
          <Order>5</Order>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add">
          <CommandLine>cmd /c net accounts /maxpwage:unlimited</CommandLine>
          <Description>Local account passwords never expire</Description>
          <Order>6</Order>
        </SynchronousCommand>
      </FirstLogonCommands>
    </component>
  </settings>
</unattend>
EOF


echo "Downloading Spice drivers..."
web_get https://www.spice-space.org/download/windows/spice-webdavd/spice-webdavd-x64-latest.msi "${VM_PATH}/unattended"
web_get https://www.spice-space.org/download/windows/vdagent/vdagent-win-0.10.0/spice-vdagent-x64-0.10.0.msi "${VM_PATH}/unattended"

echo "Making unattended.iso"
mkisofs -quiet -J -o "${VM_PATH}/unattended.iso" "${VM_PATH}/unattended/"
}
