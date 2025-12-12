#!/usr/bin/env bash

set -e

echo "Starting protocgen..."

proto_dirs=$(find ./proto -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)

for dir in $proto_dirs; do
  echo "Generating codes for: $dir"
  
  buf protoc \
  -I "proto" \
  -I "third_party/proto" \
  --gocosmos_out=plugins=interfacetype+grpc,\
Mgoogle/protobuf/any.proto=github.com/cosmos/cosmos-sdk/codec/types:. \
  $(find "${dir}" -maxdepth 1 -name '*.proto')

done

if [ -d "github.com" ]; then
  echo "Moving generated files..."
  cp -r github.com/datachainlab/anvil-cross-demo/types/* ./types
  rm -rf github.com
fi

echo "Done."
