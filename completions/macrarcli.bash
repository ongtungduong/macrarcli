# bash completion for macrarcli
# Install: source this file, or copy to /etc/bash_completion.d/ (or
# $(brew --prefix)/etc/bash_completion.d/ on Homebrew).

_macrarcli() {
    local cur flags
    cur="${COMP_WORDS[COMP_CWORD]}"
    flags="-o --dest -e --flat -l --list -t --test \
--overwrite --skip --rename -q --quiet --password \
--jobs --json --max-size --max-entries --verbose \
--version -h --help"

    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
        return
    fi
    # Otherwise complete .rar files and directories.
    COMPREPLY=( $(compgen -f -X '!*.rar' -- "$cur") $(compgen -d -- "$cur") )
}
complete -F _macrarcli macrarcli
