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
  # Version lives in the canonical CLI tool (const qeVersion).
  versionParts = builtins.split "qeVersion = \"([0-9]+\\.[0-9]+\\.[0-9]+)\"" (
    builtins.readFile ../../tools/quickemu-cli/main.go
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
      (cd "$d" && go build -trimpath -ldflags "-s -w" -o "$OLDPWD/build/$name" .)
    done
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    installManPage docs/quickget.1 docs/quickemu.1 docs/quickemu_conf.5
    install -d "$out/libexec/quickemu"
    install -Dm755 -t "$out/libexec/quickemu" build/*
    install -Dm755 -t "$out/bin" bin/quickemu bin/quickget bin/quickgetc bin/quickreport
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
