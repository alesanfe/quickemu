# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# CLI boundary: help/usage output.


function help_message() {
    #shellcheck disable=SC2016
    printf '
             _      _              _
  __ _ _   _(_) ___| | ____ _  ___| |_      ___
 / _` | | | | |/ __| |/ / _` |/ _ \ __     / __|
| (_| | |_| | | (__|   < (_| |  __/ |_    | (__
 \__, |\__,_|_|\___|_|\_\__, |\___|\__|___ \___|
    |_|                 |___/ v%s, using curl %s
--------------------------------------------------------------------------------
 Project - https://github.com/quickemu-project/quickemu
 Discord - https://wimpysworld.io/discord
--------------------------------------------------------------------------------

Usage:
  quickgetc <os> <release> [edition]
  quickgetc ubuntu 22.04

Advanced usage:
  quickgetc <arg> [path] <os> [release] [edition]
  quickgetc --download ubuntu 22.04

Arguments:
  --arch          <arch>                   : Specify the image architecture
  --download      <os> <release> [edition] : Download image; no VM configuration
  --create-config <os> [path/url]          : Create VM config for a OS image
  --open-homepage <os>                     : Open homepage for the OS
  --show          [os]                     : Show OS information
  --version                                : Show version
  --help                                   : Show this help message
-------------------------- For testing & development ---------------------------
  --url           [os] [release] [edition] : Show image URL(s)
  --check         [os] [release] [edition] : Check image URL(s)
  --list                                   : List all supported systems
  --list-csv                               : List everything in csv format
  --list-json                              : List everything in json format
--------------------------------------------------------------------------------

Supported Operating Systems:\n\n' "${QUICKGETC_VERSION}" "${CURL_VERSION}"
    os_support | fold -s -w "$(tput cols)"
}
