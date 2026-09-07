#compdef macrarcli
# zsh completion for macrarcli
# Install: place this file as _macrarcli in a directory on your $fpath
# (e.g. ~/.zsh/completions), then ensure `autoload -U compinit && compinit`.

_macrarcli() {
  _arguments -s \
    '(-o --dest)'{-o,--dest}'[destination directory for extracted files]:dir:_files -/' \
    '(-e --flat -l --list -t --test)'{-e,--flat}'[extract without preserving directory structure]' \
    '(-e --flat -l --list -t --test)'{-l,--list}'[preview archive contents without extracting]' \
    '(-e --flat -l --list -t --test)'{-t,--test}'[validate archive integrity without extracting]' \
    '(--overwrite --skip --rename)--overwrite[replace existing destination files]' \
    '(--overwrite --skip --rename)--skip[skip individual entries whose destination already exists]' \
    '(--overwrite --skip --rename)--rename[write colliding entries under a " (n)" suffixed name]' \
    '(-q --quiet)'{-q,--quiet}'[suppress progress output]' \
    '--password[password for encrypted archives]:password:' \
    '--jobs[number of archives to process concurrently]:jobs:' \
    '--json[emit a machine-readable JSON summary on stdout]' \
    '--max-size[cap total uncompressed size (e.g. 10M, 2G)]:size:' \
    '--max-entries[cap number of entries per archive]:count:' \
    '--verbose[print extra diagnostics to stderr]' \
    '--version[print version and exit]' \
    '(-h --help)'{-h,--help}'[print usage and exit]' \
    '*:rar file:_files -g "*.rar"'
}

_macrarcli "$@"
