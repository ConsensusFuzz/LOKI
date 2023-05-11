# LOKI

Blockchain consensus protocols are responsible for coordinating the nodes to make agreements on the transaction results. Their implementation bugs, including memory related and consensus logic vulnerabilities, may pose serious threats. Fuzzing is a promising technique for protocol vulnerability detection. However, existing fuzzers cannot deal with complex consensus states of distributed nodes, thus generating a large number of useless packets, inhibiting its effectiveness in reaching the deep logic of consensus protocols.

In this work, we propose LOKI, a blockchain consensus protocol fuzzing framework that detects the consensus memory related and logic bugs. LOKI senses consensus states in real-time by masquerading as a node. First, LOKI dynamically builds a state model that records the state transition of each node. After that, LOKI adaptively generates the input targets, types and contents according to the state model. With a bug analyzer, LOKI detects the consensus protocol implementation bugs with well-defined oracles. We implemented and evaluated LOKI on four widely used commercial blockchain systems, including Go-Ethereum, Facebook Diem, IBM Fabric and WeBank FISCO-BCOS. LOKI has detected 20 serious previously unknown vulnerabilities with 9 CVEs assigned. 14 of them are memory related bugs and 6 are consensus logic bugs. Compared with state-of-the-art tools such as Peach, Fluffy and Twins, LOKI improves the branch coverage by an average of 43.21%, 182.05% and 291.58%. 

# Quickstart

## LOKI for fabric

### prerequisites
Setup fabric network environment, can be found in https://hyperledger-fabric.readthedocs.io/en/release/prereqs.html.

### load LOKI-fabric image
```bash
cd fabric
docker import - smartbft/fabric-orderer:latest < LOKI-fabric.tar
```

### setup LOKI testnet & start fuzzing

```bash
cd fabric/testnet
./byfn generate
./byfn up
./byfn down
```


## LOKI for go-ethereum
### prerequisites
Setup geth network environment, can be found in https://priyalwalpita.medium.com/setting-up-the-go-ethereum-geth-environment-in-ubuntu-linux-67c09706b42.

### setup LOKI testnet & start fuzzing

```bash
cd geth/testnet
```
#### setup bootnode
```bash
../bin/bootnode -genkey bootnode.key
../bin/bootnode -nodekey bootnode.key
```
#### initialize node
```bash
for i in 1 2 3 4 5; do ../bin/geth --datadir ./node$i account new; done 
```
#### run loki-geth node

##### node1
```bash
../build/bin/geth --identity "ETH-node1" --datadir "node1" --port "30303" --maxpeers 10 --networkid 10086  --syncmode "full" --bootnodes "enode://5e49cd079bf47d4485867d4fb06a89f211b21be822a05cc8be6ce72b624aa94f4ac65e063fc4d0e62fe2342290bf5c880f6888534ae1df045f67186718d3c3f6@127.0.0.1:0?discport=30301" --mine --miner.etherbase 0xd192415624a039b24ad571f96cb438de9f0556a7 --miner.threads 1 --http --http.port 8545 console
```

##### node2
```bash
../build/bin/geth --identity "ETH-node2" --datadir "node2" --port "30304" --maxpeers 10 --networkid 10086  --syncmode "full" --bootnodes "enode://5e49cd079bf47d4485867d4fb06a89f211b21be822a05cc8be6ce72b624aa94f4ac65e063fc4d0e62fe2342290bf5c880f6888534ae1df045f67186718d3c3f6@127.0.0.1:0?discport=30301" --mine --miner.etherbase 0xd192415624a039b24ad571f96cb438de9f0556a7 --miner.threads 1 console
```

##### node3
```bash
../build/bin/geth --identity "ETH-node3" --datadir "node3" --port "30305" --maxpeers 10 --networkid 10086  --syncmode "full" --bootnodes "enode://5e49cd079bf47d4485867d4fb06a89f211b21be822a05cc8be6ce72b624aa94f4ac65e063fc4d0e62fe2342290bf5c880f6888534ae1df045f67186718d3c3f6@127.0.0.1:0?discport=30301" --mine --miner.etherbase 0xd192415624a039b24ad571f96cb438de9f0556a7 --miner.threads 1 console
```

##### node4
```bash
../build/bin/geth --identity "ETH-node4" --datadir "node4" --port "30306" --maxpeers 10 --networkid 10086  --syncmode "full" --bootnodes "enode://5e49cd079bf47d4485867d4fb06a89f211b21be822a05cc8be6ce72b624aa94f4ac65e063fc4d0e62fe2342290bf5c880f6888534ae1df045f67186718d3c3f6@127.0.0.1:0?discport=30301" --mine --miner.etherbase 0xd192415624a039b24ad571f96cb438de9f0556a7 --miner.threads 1 console
```

##### node5
```bash
../build/bin/geth --identity "ETH-node5" --datadir "node5" --port "30307" --maxpeers 10 --networkid 10086  --syncmode "full" --bootnodes "enode://5e49cd079bf47d4485867d4fb06a89f211b21be822a05cc8be6ce72b624aa94f4ac65e063fc4d0e62fe2342290bf5c880f6888534ae1df045f67186718d3c3f6@127.0.0.1:0?discport=30301" --mine --miner.etherbase 0xd192415624a039b24ad571f96cb438de9f0556a7 --miner.threads 1 console
```

#### ...

#### node_n

## LOKI for fisco

### prerequisites
Setup fisco network environment, can be found in https://fisco-bcos-documentation.readthedocs.io/en/latest/docs/installation.html

### setup LOKI testnet & start fuzzing

```bash
cd fisco/testnet
bash build_chain.sh -l 127.0.0.1:4 -p 30300,20200,8545
```
This will setup a local 4-node consortium chain of fisco-bcos, change 1 of the node to LOKI node:
```bash
# Change the start.sh and stop.sh of node0 to:
fisco_bcos=LOKI/fisco/bin/fisco-bcos
```
Then use the start-all shell to start the chain nodes:
```bash
cd nodes
./start_all.sh
```

## LOKI for Diem
### prerequisites
Just need to setup rust environment.

### setup LOKI testnet & start fuzzing
Use the following command line to start a swarm of 4 nodes with 1 LOKI node and start fuzzing.
```bash
bin/diem-swarm --fuzzer-node bin/diem-node --diem-node YOUR_PATH_TO_NORMAL_DIEM_NODE/diem-node -n 4 -t 1 -c ./tmp
```


# Troubleshooting
Create an issue for questions and bug reports.

# Contribution
We welcome your contributions to LOKI! We aim to create an open-source project that is contributed by the open-source community. For general discussions about development, please refer to the issues. 
To contact us, please send an email to xxx.

# License
[Apache-2.0 License](https://github.com/BlockFuzz/LOKI/blob/main/LICENSE)

