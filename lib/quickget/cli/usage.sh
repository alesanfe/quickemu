# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# CLI boundary: help/usage output.


function help_message() {
    #shellcheck disable=SC2016
    printf '
             _      _              _
  __ _ _   _(_) ___| | ____ _  ___| |_
 / _` | | | | |/ __| |/ / _` |/ _ \ __|
| (_| | |_| | | (__|   < (_| |  __/ |_
 \__, |\__,_|_|\___|_|\_\__, |\___|\__|
    |_|                 |___/ v%s, using curl %s
--------------------------------------------------------------------------------
 Project - https://github.com/quickemu-project/quickemu
 Discord - https://wimpysworld.io/discord
--------------------------------------------------------------------------------

Usage:
  quickget <os> <release> [edition]
  quickget ubuntu 22.04

Advanced usage:
  quickget <arg> [path] <os> [release] [edition]
  quickget --download ubuntu 22.04

Arguments:
   --arch           <arch>                    : Set architecture (arm64, aarch64, amd64, x86_64)
   --download       <os> <release> [edition]  : Download image; no VM configuration
   --create-config  <os> [path/url] [flags]   : Create VM config for an OS image
   --insecure                                 : Skip SSL certificate verification (or set QUICKGET_INSECURE=1)
   --cloud-init                               : Generate a NoCloud seed.iso for the downloaded VM
   --unattended                               : Configure unattended install (ubuntu-server)
   --update        <os>                        : Show newest upstream release vs local VMs
   --open-homepage  <os>                      : Open homepage for the OS
   --show           [os]                      : Show OS information
   --version                                  : Show version
   --help                                     : Show this help message
------------------------------------ Flags -------------------------------------
--create-config:
  --disable-unattended                        : Force quickget not to set up an unattended installation
-------------------------- For testing & development ---------------------------
   --url            [os] [release] [edition]  : Show image URL(s)
   --check          [os] [release] [edition]  : Check image URL(s)
   --check-all-arch [os] [release] [edition]  : Check downloads for all architectures (amd64 and arm64)
   --list                                     : List all supported systems
   --list-csv                                 : List everything in csv format
   --list-json                                : List everything in json format
--------------------------------------------------------------------------------

Supported Operating Systems:\n\n' "$(${QUICKEMU} --version)" "${CURL_VERSION}"
    os_support | fmt -w 80
}
