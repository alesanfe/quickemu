{
  lib,
  installShellFiles,
  makeWrapper,
  stdenv,
  testers,
  go,
  mesa-demos,
  pciutils,
  procps,
  qemu_full,
  samba,
  socat,
  spice-gtk,
  swtpm,
  usbutils,
  util-linux,
  xdg-user-dirs,
  xrandr,
  zsync,
  OVMF ? null,
  OVMFFull ? null,
  virtiofsd ? null,
}:
let
  # Runtime tools the Go backend execs: QEMU itself, its companion
  # daemons (swtpm, virtiofsd, qemu-bridge-helper via util-linux),
  # optional accelerators (zsync delta downloads) and viewers
  # (spice-gtk/xrandr). Catalog data + schemas ship under share/quickemu.
  runtimePaths = [
    pciutils
    procps
    qemu_full
    samba
    swtpm
    util-linux
    xrandr
    zsync
  ]
  ++ lib.optionals stdenv.hostPlatform.isLinux [
    mesa-demos
    usbutils
    xdg-user-dirs
    virtiofsd
  ];
  # Version lives in the shared library (qelib.QEVersion).
  versionParts = builtins.split "QEVersion = \"([0-9]+\\.[0-9]+\\.[0-9]+)\"" (
    builtins.readFile ../../tools/qelib/qelib.go
  );
  version = builtins.elemAt (builtins.elemAt versionParts 1) 0;
in
stdenv.mkDerivation (finalAttrs: {
  pname = "quickemu";
  inherit version;
  src = lib.cleanSource ../..;

  # All 15 modules share go.work + vendor/ — builds are fully offline.
  env.GOFLAGS = "-mod=vendor";
  env.GOWORK = "on";
  env.CGO_ENABLED = "0";

  nativeBuildInputs = [
    go
    makeWrapper
    installShellFiles
  ];

  buildPhase = ''
    runHook preBuild
    mkdir -p build
    for d in tools/*/; do
      name=$(basename "$d")
      [ -f "$d/go.mod" ] || continue
      case "$name" in qelib|dev|qespec) continue;; esac
      (cd "$d" && go build -trimpath -ldflags "-s -w" -o "$OLDPWD/build/$name" .)
    done
    # Public entries are the cli binaries under their canonical names.
    cp build/quickemu-cli  build/quickemu
    cp build/quickget-cli  build/quickget
    cp build/quickgetc     build/quickgetc
    cp build/quickreport   build/quickreport
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    installManPage docs/quickget.1 docs/quickemu.1 docs/quickemu_conf.5
    install -d "$out/libexec/quickemu"
    install -Dm755 -t "$out/libexec/quickemu" build/quickemu-* build/quickget-*
    install -Dm755 -t "$out/bin" build/quickemu build/quickget build/quickgetc build/quickreport
    install -d "$out/share/quickemu"
    cp -r schemas catalog "$out/share/quickemu/"

    # spice-gtk needs to be put in suffix so that when virtualisation.spiceUSBRedirection
    # is enabled, the wrapped spice-client-glib-usb-acl-helper is used
    for f in quickget quickgetc quickemu quickreport; do
      wrapProgram $out/bin/$f \
        --prefix PATH : "$out/libexec/quickemu" \
        --prefix PATH : "${lib.makeBinPath runtimePaths}" \
        --suffix PATH : "${lib.makeBinPath [ spice-gtk ]}" \
        ${lib.optionalString (OVMF != null || OVMFFull != null) ''
        --set-default QE_FIRMWARE_DIR "${if OVMFFull != null then "${OVMFFull.fd}/FV" else "${OVMF.fd}/FV"}" \
        ''}
    done

    runHook postInstall
  '';

  passthru.tests = testers.testVersion { package = finalAttrs.finalPackage; };

  meta = {
    description = "Quickly create and run optimised Windows, macOS and Linux virtual machines";
    homepage = "https://github.com/quickemu-project/quickemu";
    changelog = "https://github.com/quickemu-project/quickemu/releases/tag/${finalAttrs.version}";
    mainProgram = "quickemu";
    license = lib.licenses.mit;
    maintainers = with lib.maintainers; [
      fedx-sudo
      flexiondotorg
    ];
  };
})
