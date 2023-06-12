#!/bin/bash -x


export GOROOT=$(go env GOROOT)
export GOPATH=$(go env GOPATH)
mkdir -p $GOPATH/src/github.com/hyperledger/
cd ..
echo "[[[[" `pwd` "]]]]"
mv fabric $GOPATH/src/github.com/hyperledger/
cd $GOPATH/src/github.com/hyperledger/fabric


