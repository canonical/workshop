set -ex

# zsh completion functions use shell builtins to return their results. Rather
# than attempting to override the right set of functions, we can just simulate
# an interactive session and check the right command was executed. This is
# enough to test the packaged completion script after sourcing it. More
# complex scenarios can be tested using unit tests or bash.
workshop connections ws-comp | grep -Eq '^mount[[:space:]]+ws-comp/test-sdk-mount:one[[:space:]]+ws-comp/system:mount[[:space:]]+-$'
touch ~/.zshrc
printf 'autoload -Uz compinit; compinit\nsource /snap/workshop/current/completions/workshop.zsh\nsource /snap/workshop/current/completions/sdk.zsh\n%s\t\nexit\n' 'workshop disconnect ws-comp/test-sdk-mount:o' | script -qc zsh /dev/null > /dev/null
workshop connections ws-comp | grep -Eq '^mount[[:space:]]+ws-comp/test-sdk-mount:one[[:space:]]+-[[:space:]]+-$'
