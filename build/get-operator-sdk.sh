#!/bin/bash -xe

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
	ARCH=$(uname -m)
	# For darwin arm64 and versions < v1.23.0, use x86_64 since arm64 binaries don't exist
	if [[ "$ARCH" == "arm64" ]]; then
		# Compare version with v1.23.0 (remove 'v' prefix if present)
		VERSION_NUM="${VERSION#v}"
		if printf '%s\n%s\n' "1.23.0" "$VERSION_NUM" | sort -V | head -n1 | grep -q "1.23.0"; then
			# Version is >= 1.23.0, keep arm64
			URL="$BASE_URL/$VERSION/operator-sdk-$VERSION-arm64-apple-darwin"
		else
			# Version is < 1.23.0, use x86_64
			URL="$BASE_URL/$VERSION/operator-sdk-$VERSION-x86_64-apple-darwin"
		fi
	else
		URL="${URL%-linux-gnu}-apple-darwin"
	fi
fi

# Fetch the binary
curl -Lo operator-sdk "$URL"
curl -Lo operator-sdk.asc "$URL.asc"

# Verify
gpg --keyserver keyserver.ubuntu.com --recv-key "$(gpg --verify operator-sdk.asc operator-sdk 2>&1 /dev/null | grep RSA | awk '{ print $NF }')"
gpg --verify operator-sdk.asc operator-sdk

chmod +x ./operator-sdk
