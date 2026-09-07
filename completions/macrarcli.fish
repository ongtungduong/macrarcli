# fish completion for macrarcli
# Install: copy to ~/.config/fish/completions/macrarcli.fish

complete -c macrarcli -f

complete -c macrarcli -s o -l dest      -r -d 'destination directory for extracted files'
complete -c macrarcli -s e -l flat         -d 'extract without preserving directory structure'
complete -c macrarcli -s l -l list         -d 'preview archive contents without extracting'
complete -c macrarcli -s t -l test         -d 'validate archive integrity without extracting'
complete -c macrarcli      -l overwrite    -d 'replace existing destination files'
complete -c macrarcli      -l skip         -d 'skip individual entries whose destination already exists'
complete -c macrarcli      -l rename       -d 'write colliding entries under a " (n)" suffixed name'
complete -c macrarcli -s q -l quiet        -d 'suppress progress output'
complete -c macrarcli      -l password  -r -d 'password for encrypted archives'
complete -c macrarcli      -l jobs      -r -d 'number of archives to process concurrently'
complete -c macrarcli      -l json         -d 'emit a machine-readable JSON summary'
complete -c macrarcli      -l max-size  -r -d 'cap total uncompressed size (e.g. 10M, 2G)'
complete -c macrarcli      -l max-entries -r -d 'cap number of entries per archive'
complete -c macrarcli      -l verbose      -d 'print extra diagnostics to stderr'
complete -c macrarcli      -l version      -d 'print version and exit'
complete -c macrarcli -s h -l help         -d 'print usage and exit'

# Positional arguments: .rar files.
complete -c macrarcli -k -a '(__fish_complete_suffix .rar)'
