{
  lib,
  mkShell,
  pkgs,
  stdenv,
  OVMF ? null,
  OVMFFull ? null,
}:
mkShell {
  packages =
    with pkgs;
    (
      [
        # Toolchain
        go
        gnumake
        # QEMU runtime + companions the Go tools exec
        qemu_full
        swtpm
        samba
        zsync
        # qespec e2e probes QMP sockets via Go net; socat kept for
        # ad-hoc debugging of monitor sockets.
        socat
        pciutils
        procps
        util-linux
        xorg.xrandr
        spice-gtk
      ]
      ++ lib.optionals stdenv.hostPlatform.isLinux [
        mesa-demos
        usbutils
        xdg-user-dirs
      ]
      ++ lib.optionals (OVMF != null && OVMFFull != null) [
        OVMF
        OVMFFull
      ]
    );

  inputsFrom = with pkgs; [
    git
  ];

  # Firmware search override — the Go launcher's firmwareFile() reads
  # QE_FIRMWARE_DIR first, so nix-store OVMF paths plug in without
  # patching sources.
  QE_FIRMWARE_DIR = lib.optionalString (OVMFFull != null) "${OVMFFull.fd}/FV";

  shellHook = ''
    echo "**********************************************************************"
    echo "* 'direnv reload' to update '.direnv/bin' for testing           *"
    echo "**********************************************************************"
    mkdir -p $PWD/.direnv/bin
    for f in quickemu quickget quickgetc quickreport; do
      cp bin/$f $PWD/.direnv/bin/$f
      chmod +x $PWD/.direnv/bin/$f
    done
    # Entries resolve ../tools/<t>/<t> from their own dir.
    ln -sfn "$PWD/tools" "$PWD/.direnv/tools"
    # Build the Go tools once so the bin/ entries work immediately.
    if command -v go >/dev/null; then
      go run ./tools/dev build >/dev/null 2>&1 || \
        echo "hint: run 'go run ./tools/dev build' to build the Go tools"
    fi
  '';
}
