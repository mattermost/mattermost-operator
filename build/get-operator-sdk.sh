#!/bin/bash

BASE_URL="https://github.com/operator-framework/operator-sdk/releases/download"

# check if version is specified
if [ "$#" -ne 1 ]
then
   echo "version is required"
   exit 1
fi
VERSION="$1"

# cd into the build directory
cd "$(dirname "${0}")" || exit

# Check if binary exists and is of correct version
if [ -f ./operator-sdk ] && ./operator-sdk version | grep "$VERSION"
then
	exit 0
fi

# Download Operator SDK
if [[ "$OSTYPE" == "darwin"* ]]
then
	curl -Lo operator-sdk "$BASE_URL/$VERSION/operator-sdk-$VERSION-$(uname -m)-apple-darwin"
elif [[ "$OSTYPE" == "linux-gnu"* ]]
then
	curl -Lo operator-sdk "$BASE_URL/$VERSION/operator-sdk-$VERSION-$(uname -m)-linux-gnu"
fi

chmod +x ./operator-sdk
