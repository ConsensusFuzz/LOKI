#!/usr/bin/env bash -e

echo "go version:"
go version

echo "Building docker images..."
git checkout origin/release-1.4-BFT-3

echo "Building on top of $(git branch | sed -n '/\* /s///p')"

git log -1 --stat

make orderer-docker peer-docker tools-docker 

echo "Tagging images..."
docker tag hyperledger/fabric-peer:latest smartbft/fabric-peer:rotation
docker tag hyperledger/fabric-orderer:latest smartbft/fabric-orderer:rotation
docker tag hyperledger/fabric-tools:latest smartbft/fabric-tools:rotation

echo "Logging in to dockerhub..."
echo ${DOCKER_PASSWORD} | docker login -u smartbft --password-stdin

echo "Pushing to dockerhub..."

docker push smartbft/fabric-peer:rotation
docker push smartbft/fabric-orderer:rotation
docker push smartbft/fabric-tools:rotation

cat /dev/null > ~/.docker/config.json
