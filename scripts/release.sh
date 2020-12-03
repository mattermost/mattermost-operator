#!/usr/bin/env bash

set -o errexit
set -o pipefail

export OPERATOR_TAG=""

check_github() {
  local valid_upstream
  local valid_repo

  valid_repo="mattermost/mattermost-operator"

  valid_upstream=$(git config --get remote.upstream.url)

  echo "$valid_upstream"
  if [[ "$valid_upstream" =~ .*"$valid_repo".* ]]; then
    echo "It is using upstream repo. valid!"
  else
    echo "Please set the upstream"
    exit 1
  fi
}

check_status() {
  if [[ $(git diff --stat) != '' ]]; then
  echo 'Git repo is dirty, please use a clean repo'
  exit 1
  fi
}

bump_version() {
  echo "Bumping the version file"
  sed -i.bak -e "s/var version = .*/var version = \"$OPERATOR_TAG\"/g" -- version/version.go && rm -- version/version.go.bak

  echo "Bumping the image template"
  sed -i.bak -e "s/image: .*/image: mattermost\/mattermost-operator:v$OPERATOR_TAG/g" docs/mattermost-operator/mattermost-operator.yaml && rm -- docs/mattermost-operator/mattermost-operator.yaml.bak

  echo "Commiting and pushing to upstream"
  git commit -am "Bump operator to version $OPERATOR_TAG"
  git push upstream master
}

tag_version() {
  echo "pushing tag $OPERATOR_TAG"
  git tag v$OPERATOR_TAG
  git push upstream v$OPERATOR_TAG
}

# setup kind, build kubernetes, create a cluster, run the e2es
main() {
  while test $# -gt 0; do
    case "$1" in
      -h|--help)
        echo "release mattermost operator script"
        echo " "
        echo "release.sh [options]"
        echo " "
        echo "options:"
        echo "-h, --help                show brief help"
        echo "-t, --tag=TAG             new tag to release the operator"
        exit 0
        ;;
      -t)
        shift
        if test $# -gt 0; then
        OPERATOR_TAG=$1
        else
          echo "no tag specified"
          exit 1
        fi
        shift
        ;;
      --tag*)
        OPERATOR_TAG=$(sed -e 's/^[^=]*=//g' <<< "$1" )
        shift
        ;;
      *)
        break
        ;;
    esac
  done

  if [ "$OPERATOR_TAG" == "" ]; then
    echo 'A TAG is required, use -t <TAG> or --tag=<TAG>' >&2
    exit 1
  fi

  OPERATOR_TAG=$( echo "$OPERATOR_TAG" | tr -d v)

  echo "Will release Mattermost Operator TAG ${OPERATOR_TAG}"

  check_github

  check_status

  bump_version

  tag_version

  echo "Done. Watch CircleCI job now :)"
}

main "$@"