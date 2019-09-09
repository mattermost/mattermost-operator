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

# Linux binary is set as default, if Mac OS is detected the URL will
# be overwritten
URL="$BASE_URL/$VERSION/operator-sdk-$VERSION-$(uname -m)-linux-gnu"

if [[ "$OSTYPE" == "darwin"* ]]
then
	URL="${URL%-linux-gnu}-apple-darwin"
fi

# Fetch the binary
curl -Lo operator-sdk "$URL"
curl -Lo operator-sdk.asc "$URL.asc"

# Verify
gpg --keyserver keyserver.ubuntu.com --recv-key "$(gpg --verify operator-sdk.asc operator-sdk 2>&1 /dev/null | grep RSA | awk '{ print $NF }')"
gpg --verify operator-sdk.asc operator-sdk

chmod +x ./operator-sdk
