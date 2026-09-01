set -e

do_complete() {
  set +x

  COMP_LINE="$*"
  COMP_POINT="${#COMP_LINE}"
  COMP_KEY=9  # ASCII code for tab
  COMP_TYPE=33  # ASCII code for !
  COMP_WORDS=("$@")
  COMP_CWORD="${#COMP_WORDS[@]}"
  ((COMP_CWORD--))

  "$func" "${COMP_WORDS[0]}" "${COMP_WORDS[-1]}" "${COMP_WORDS[-2]}" 2>/dev/null || true

  set -x
}

source /usr/share/bash-completion/bash_completion
_completion_loader sdk || true
_completion_loader workshop || true

set -x

func='__start_workshop'
complete -p workshop | grep -qw "$func"

# The completion functionality is well covered in unit tests. We check some
# small examples here that each use different code paths.

echo "Test launch completion"
do_complete workshop launch o
[ "$COMPREPLY" = off ]

echo "Test exec --env completion"
export OBSCURE_ENVIRONMENT_VARIABLE_WITH_NO_SHARED_PREFIX=1
do_complete workshop exec --env OBSCURE_ENVIRONMENT_VARIABLE_WITH_NO_SHARED_PR
[ "$COMPREPLY" = 'OBSCURE_ENVIRONMENT_VARIABLE_WITH_NO_SHARED_PREFIX' ]

echo "Test connect completion"
do_complete workshop connect w
[ "$COMPREPLY" = 'ws-comp/test-sdk-desktop:desktop' ]

echo "Test remount dir completion"
mkdir -p ~/tmp/tmp
do_complete workshop remount ws-comp/test-sdk-mount:one "$HOME/tmp/t"
[ "$COMPREPLY" = "$HOME/tmp/tmp" ]

echo "Test stop completion"
do_complete workshop stop w
[ "$COMPREPLY" = ws-comp ]

echo "Test project completion"
mkdir -p empty-dir-with-no-workshops
do_complete workshop launch --project ''
printf '%s\0' "${COMPREPLY[@]}" | grep -Fqxz empty-dir-with-no-workshops
do_complete workshop refresh --project ''
printf '%s\0' "${COMPREPLY[@]}" | grep -Fqxz .
pushd .. >/dev/null
do_complete workshop launch --project compl
[ "$COMPREPLY" = completion ]
do_complete workshop refresh --project compl
[ "$COMPREPLY" = completion ]
do_complete workshop refresh --project "$PWD/compl"
[ "$COMPREPLY" = "$PWD/completion" ]
popd >/dev/null

func='__start_sdk'
complete -p sdk | grep -qw "$func"

echo "Test base completion"
do_complete sdk info --base ubuntu@24.0
[ "$COMPREPLY" = ubuntu@24.04 ]

echo "Test arch completion"
do_complete sdk info --arch s3
[ "$COMPREPLY" = s390x ]
