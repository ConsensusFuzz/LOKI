use crate::{
    metrics_safety_rules::MetricsSafetyRules,
    network::NetworkSender,
    network_interface::{ConsensusMsg,ConsensusNetworkSender},
};
use csv::Writer;
use std::sync::atomic::{AtomicUsize,Ordering};
use rand::seq::SliceRandom; 
use rand::Rng;
use diem_crypto::hash::HashValue;
use diem_infallible::{checked, Mutex};
use std::{
    sync::{atomic::AtomicU64, Arc},
    time::Duration,
};
// use tokio::time;
use safety_rules::ConsensusState;
use safety_rules::TSafetyRules;
use channel::{self, diem_channel, message_queues::QueueStyle};
use consensus_types::{block_data::BlockData,common::{Author, Round},block::{Block}, block_retrieval::{BlockRetrievalRequest,BlockRetrievalResponse,BlockRetrievalStatus}, epoch_retrieval::EpochRetrievalRequest, experimental::{commit_decision::{self, CommitDecision}, commit_vote::CommitVote}, proposal_msg::ProposalMsg, quorum_cert::QuorumCert, sync_info::SyncInfo, vote::Vote, vote_data::VoteData, vote_msg::VoteMsg};
use diem_types::{
    epoch_change::EpochChangeProof,
    ledger_info::{LedgerInfo, LedgerInfoWithSignatures},
    validator_signer::ValidatorSigner,
    validator_verifier::ValidatorVerifier,
    block_info::BlockInfo,
    on_chain_config::ValidatorSet
};
use diem_crypto::hash::CryptoHash;
use diem_logger::prelude::*;
use network::{
    peer_manager::{ConnectionRequestSender, PeerManagerRequestSender},
    protocols::network::NewNetworkSender,
};
use std::{collections::{BTreeMap,HashMap}};
use std::{thread, time};

// static FUZZ_ITERATION: AtomicUsize = AtomicUsize::new(0);

pub fn start_fuzzer(network_ : NetworkSender, LAST_MSG : &mut Vec<ConsensusMsg>, safety_rules:Arc<Mutex<MetricsSafetyRules>>,author:Author){
    // let mut network = create_network_for_fuzzing(validators);
    thread::sleep(time::Duration::from_millis(10000));
    // let mut FUZZ_ITERATION : f32 = 0.0;
    // let mut cur_iteration : f32 = 0.0;
    let mut network = network_;
    // let mut _time: u32 = 0;
    info!(
        "Crusader fuzzer has started!!!"
    );
    let mut sy_time = std::time::SystemTime::now();
    // fuzzing loop
    loop{
        // println!("Fuzz once!!!!");
        // FUZZ_ITERATION = FUZZ_ITERATION + 1.0;
        // if std::time::SystemTime::now().duration_since(sy_time).unwrap().as_secs() >= 60 {
        //     _time = _time + 1;
        //     let mut file = std::fs::OpenOptions::new()
        //         .write(true)
        //         .create(true)
        //         .append(true)
        //         .open("fuzz_iteration_diem.csv")
        //         .unwrap();
        //     let mut wtr = Writer::from_writer(file);
        //     wtr.write_record(&[_time.to_string(), format!("{:.2}", (FUZZ_ITERATION - cur_iteration)/60.0)]);
        //     wtr.flush();
        //     FUZZ_ITERATION = 0.0;
        //     sy_time = std::time::SystemTime::now();
        // }
        // info!(
        //     "Crusader fuzzer starts its loop!!!"
        // );
        if !LAST_MSG.is_empty(){
            info!(
                "Crusader fuzzer sent based on existing packets from {:?} packages!!!",
                LAST_MSG.len()
            );
            // choose a message to mutate
            let chosen_msg = LAST_MSG.choose(&mut rand::thread_rng()).unwrap();
            //let chosen_msg = &LAST_MSG[LAST_MSG.len()-1];
            match chosen_msg{
                ConsensusMsg::ProposalMsg(m) => {
                    info!("Crusader fuzzer has chose the proposalMsg");
                    let proposal = mutate_proposal(*m.clone(),safety_rules.clone(),author);
                    network.send_to_others(ConsensusMsg::ProposalMsg(Box::new(proposal)));
                }
                ConsensusMsg::SyncInfo(m) => {
                    info!("Crusader fuzzer has chose the syncinfo");
                    network.send_to_others(ConsensusMsg::SyncInfo(m.clone()));
                }
                ConsensusMsg::VoteMsg(m) => {
                    info!("Crusader fuzzer has chose the VoteMsg");
                    let vote_msg = mutate_vote(*m.clone());
                    network.send_to_others(ConsensusMsg::VoteMsg(Box::new(vote_msg)));
                }
                ConsensusMsg::CommitVoteMsg(m) => {
                    info!("Crusader fuzzer has chose the CommitMsg");
                    network.send_to_others(ConsensusMsg::CommitVoteMsg(m.clone()));
                }
                ConsensusMsg::CommitDecisionMsg(m) => {
                    info!("Crusader fuzzer has chose the CommitDecisionMsg");
                    network.send_to_others(ConsensusMsg::CommitDecisionMsg(m.clone()));
                } 
            //    ConsensusMsg::EpochChangeProof(m) => {
            //        info!("Crusader fuzzer has chose the epochChangeProofMsg");
            //        let request = mutate_EpochChangeProof(*m.clone());
            //        network.send_to_others(ConsensusMsg::EpochChangeProof(Box::new(request)));
            //    }
            //    ConsensusMsg::EpochRetrievalRequest(m) => {
            //        info!("Crusader fuzzer has chose the epochRetrievalrequest");
            //        let request = mutate_EpochRetrievalRequest(*m.clone());
            //        network.send_to_others(ConsensusMsg::EpochRetrievalRequest(Box::new(request)));
            //    }
            //    ConsensusMsg::BlockRetrievalRequest(m) => {
            //        info!("Crusader fuzzer has chose the blockRetrievalRequest");
            //        let request = mutate_BlockRetrievalRequest(*m.clone());
            //        network.send_to_others(ConsensusMsg::BlockRetrievalRequest(Box::new(request)));
            //    }
            //    ConsensusMsg::BlockRetrievalResponse(m) => {
            //        info!("Crusader fuzzer has chose the blockretrievalresponse");
            //        let response = mutate_BlockRetrievalResponse(*m.clone());
            //        network.send_to_others(ConsensusMsg::BlockRetrievalResponse(Box::new(response)));
            //    }
                _ => {
                    continue;
                }
            }
            if  LAST_MSG.len() > 10{
                LAST_MSG.clear();
            }
        }
        // else{
        //     let proposal = generate_proposal();
        //     // info!(
        //     //     "Crusader fuzzer generates a proposals"
        //     // );
        //     // network.broadcast(ConsensusMsg::ProposalMsg(Box::new(proposal)));
        //     network.send_to_others(ConsensusMsg::ProposalMsg(Box::new(proposal)));
        //     // let block_retrieval_request = generate_BlockRetrievalRequest();
        //     // // network.broadcast(ConsensusMsg::BlockRetrievalRequest(Box::new(block_retrieval_request)));
        //     // network.send_to_others(ConsensusMsg::BlockRetrievalRequest(Box::new(block_retrieval_request)));
        //     // let block_retrieval_response= generate_BlockRetrievalResponse();
        //     // // network.broadcast(ConsensusMsg::BlockRetrievalResponse(Box::new(block_retrieval_response)));
        //     // network.send_to_others(ConsensusMsg::BlockRetrievalResponse(Box::new(block_retrieval_response)));
        //     // let epoch_retrieval_request = generate_EpochRetrievalRequest();
        //     // // network.broadcast(ConsensusMsg::EpochRetrievalRequest(Box::new(epoch_retrieval_request)));
        //     // network.send_to_others(ConsensusMsg::EpochRetrievalRequest(Box::new(epoch_retrieval_request)));
        //     let sync_info = generate_SyncInfo();
        //     // network.broadcast(ConsensusMsg::SyncInfo(Box::new(sync_info)));
        //     network.send_to_others(ConsensusMsg::SyncInfo(Box::new(sync_info)));
        //     // let epoch_change_proof = generate_EpochChangeProof();
        //     // // network.broadcast(ConsensusMsg::EpochChangeProof(Box::new(epoch_change_proof)));
        //     // network.send_to_others(ConsensusMsg::EpochChangeProof(Box::new(epoch_change_proof)));
        //     let vote_msg = generate_VoteMsg();
        //     // network.broadcast(ConsensusMsg::VoteMsg(Box::new(vote_msg)));
        //     network.send_to_others(ConsensusMsg::VoteMsg(Box::new(vote_msg)));
        //     let commit_vote_msg = generate_CommitVoteMsg();
        //     // network.broadcast(ConsensusMsg::CommitVoteMsg(Box::new(commit_vote_msg)));
        //     network.send_to_others(ConsensusMsg::CommitVoteMsg(Box::new(commit_vote_msg)));
        //     let commit_decision_msg = generate_CommitDecisionMsg();
        //     // network.broadcast(ConsensusMsg::CommitDecisionMsg(Box::new(commit_decision_msg)));
        //     network.send_to_others(ConsensusMsg::CommitDecisionMsg(Box::new(commit_decision_msg)));
        // } 
        // sleep for 1 second
        let sleep_time = time::Duration::from_millis(100);
        thread::sleep(sleep_time);
        // if (FUZZ_ITERATION as u32) % 50000 == 0{
        //     let sleep_time = time::Duration::from_millis(200);
        //     thread::sleep(sleep_time);
        // }
       
    }
}


// Creates an RoundManager for fuzzing
// fn create_network_for_fuzzing(validators : &ValidatorSet) -> NetworkSender {
//     // signer is re-used accross fuzzing runs
//     let signer = ValidatorSigner::from_int(1);

//     let validator = ValidatorVerifier::new_single(signer.author(), signer.public_key());
//     // let validator_set = (&validator).into();
//     // TODO: mock channels
//     let (network_reqs_tx, _network_reqs_rx) = diem_channel::new(QueueStyle::FIFO, 8, None);
//     let (connection_reqs_tx, _) = diem_channel::new(QueueStyle::FIFO, 8, None);
//     let network_sender = ConsensusNetworkSender::new(
//         PeerManagerRequestSender::new(network_reqs_tx),
//         ConnectionRequestSender::new(connection_reqs_tx),
//     );
//     let (self_sender, _self_receiver) = channel::new_test(8);

//     let network = NetworkSender::new(
//         signer.author(),
//         network_sender,
//         self_sender,
//         validators.into() ,
//     );

//     network
// }

fn generate_proposal() -> ProposalMsg{
    let ledger_info = LedgerInfo::mock_genesis(None);
    let previous_qc = QuorumCert::certificate_for_genesis_from_ledger_info(
        &ledger_info,
        Block::make_genesis_block_from_ledger_info(&ledger_info).id(),
    );
    let mut rng = rand::thread_rng();
    let round : u64 = rng.gen();
    let timestamp_usecs: u64 = rng.gen();
    let validator_seed: u8 = rng.gen();
    let proposal = ProposalMsg::new(
        Block::new_proposal(vec![], round, timestamp_usecs, previous_qc.clone(), &ValidatorSigner::from_int(validator_seed)),
        SyncInfo::new(previous_qc.clone(), previous_qc, None),
    );
    proposal
}

pub fn mutate_proposal(cur_pro: ProposalMsg,safety_rules: Arc<Mutex<MetricsSafetyRules>>,author: Author) -> ProposalMsg{
    let mut rng = rand::thread_rng();
   // let timestamp_usecs: u64 = rng.gen();
    let mut timestamp_usecs: u64 = cur_pro.proposal().timestamp_usecs();
    let temp4: u64 = rng.gen();
    if temp4 % 4 == 0{
        timestamp_usecs += temp4 % 10000;
    }
    else if temp4 % 4 ==1 && timestamp_usecs > 10000{
        timestamp_usecs -= temp4 % 10000;
    }
    else if temp4 % 4 ==2{
        timestamp_usecs += temp4 % 100;
    }
    let validator_seed: u8 = rng.gen();

    let block = cur_pro.proposal();
    let mut round = block.round();
    let temp : u64 = rng.gen();
    if temp % 3 == 0{
        round += 10; 
    }
    else if temp % 3 ==1 && round > 10{
        round -=10;
    }    
    else if temp % 3 == 2{
        round = temp;
    }
    // round = temp;

    //let author = block.block_data().author().unwrap();
    let sync_info = cur_pro.sync_info();
    let payload = block.payload().unwrap();
    let quorum_cert = block.quorum_cert();
    //let block_data = block.block_data();
    let block_data = BlockData::new_proposal(payload.to_vec(),author,round,timestamp_usecs,quorum_cert.clone());
    let sig = safety_rules.lock().sign_proposal(&block_data).unwrap();
    let signed_proposal =
            Block::new_proposal_from_block_data_and_signature(block_data, sig);
    let proposal = ProposalMsg::new(
        //Block::new_proposal_with_sig(payload.clone(), round, timestamp_usecs, author, quorum_cert.clone(),sig.clone()),
        signed_proposal,
        sync_info.clone(),
    );
    proposal

    // let ledger_info = cur_pro.ledger_info();
}

fn generate_BlockRetrievalRequest() -> BlockRetrievalRequest{
    let block_id = HashValue::random();
    let mut rng = rand::thread_rng();
    let block_num : u64 = rng.gen();
    // let block_num = 1;
    BlockRetrievalRequest::new(block_id, block_num)
}

pub fn mutate_BlockRetrievalRequest(cur_pro: BlockRetrievalRequest) -> BlockRetrievalRequest{
    let block_id = cur_pro.block_id();
    let mut rng = rand::thread_rng();
    let temp : u64 = rng.gen();
    let mut block_num : u64 = cur_pro.num_blocks();
    if temp % 4 == 0 {
        block_num += 1;
    }
    else if temp % 4 == 1 {
        block_num -= 1;
    }
    else if temp % 4 == 2 {
        block_num = temp;
    }
    //block_num = temp;
    BlockRetrievalRequest::new(block_id, block_num)
}

fn generate_BlockRetrievalResponse() -> BlockRetrievalResponse{
    let mut rng = rand::thread_rng();
    let temp : u64 = rng.gen();
    let mut status: BlockRetrievalStatus;
    if temp % 3 == 0{
        status = BlockRetrievalStatus::IdNotFound;
    }
    else if temp % 3 ==1 {
        status = BlockRetrievalStatus::NotEnoughBlocks;
    }
    else{
        status = BlockRetrievalStatus::Succeeded;
    }
    BlockRetrievalResponse::new(status, vec![])
}

pub fn mutate_BlockRetrievalResponse(cur_pro:BlockRetrievalResponse) -> BlockRetrievalResponse{
    let mut rng = rand::thread_rng();
    let temp : u64 = rng.gen();
    let mut status: BlockRetrievalStatus;
    if temp % 3 == 0{
        status = BlockRetrievalStatus::IdNotFound;
    }
    else if temp % 3 ==1 {
        status = BlockRetrievalStatus::NotEnoughBlocks;
    }
    else{
        status = BlockRetrievalStatus::Succeeded;
    }
    let blocks = cur_pro.blocks();
    BlockRetrievalResponse::new(status, blocks.to_vec())
}

fn generate_EpochRetrievalRequest() -> EpochRetrievalRequest{
    let mut rng = rand::thread_rng();
    let start: u64 = rng.gen();
    let end: u64 = rng.gen();
    let new_msg = EpochRetrievalRequest {
        start_epoch: start,
        end_epoch: end,
    };
    new_msg
}

pub fn mutate_EpochRetrievalRequest(cur_req: EpochRetrievalRequest) -> EpochRetrievalRequest{
    let mut rng = rand::thread_rng();
    let mut start: u64 = rng.gen();
    let mut end: u64 = rng.gen();
    if start % 3 == 0{
        start = cur_req.start_epoch;
    }
    else if start % 3 ==1 {
        start = start % cur_req.start_epoch;
    }
    else {
        start = start % cur_req.end_epoch;
    }
    if end % 3 == 0{
        end = cur_req.end_epoch;
    }
    else if end % 3 == 1{
        end = end % cur_req.start_epoch;
    }
    else {
        end = end % cur_req.end_epoch;
    }
    //start = rng.gen();
    //end = rng.gen();
    let new_msg = EpochRetrievalRequest {
        start_epoch: start,
        end_epoch: end,
    };
    new_msg
}

pub fn generate_SyncInfo() -> SyncInfo{
    let ledger_info = LedgerInfo::mock_genesis(None);
    let previous_qc = QuorumCert::certificate_for_genesis_from_ledger_info(
        &ledger_info,
        Block::make_genesis_block_from_ledger_info(&ledger_info).id(),
    );
    SyncInfo::new(previous_qc.clone(), previous_qc, None)
}

fn generate_EpochChangeProof() -> EpochChangeProof{
    let ledger_info = LedgerInfo::mock_genesis(None);
    let mut more: bool = false;
    let mut rng = rand::thread_rng();
    let temp:u8 = rng.gen();
    if temp % 2 == 0{
        more = true;
    } 
    let lis = LedgerInfoWithSignatures::new(ledger_info, BTreeMap::new());
    EpochChangeProof::new(vec![lis],more)
}

pub fn mutate_EpochChangeProof(proof: EpochChangeProof) -> EpochChangeProof{
    let mut more: bool = false;
    let mut rng = rand::thread_rng();
    let temp:u8 = rng.gen();
    if temp % 2 == 0{
        more = true;
    } 
    let lis = proof.ledger_info_with_sigs();
    EpochChangeProof::new(lis,more)
}

fn generate_VoteMsg() -> VoteMsg{
    let mut rng = rand::thread_rng();
    let round : u64 = rng.gen();
    let temp: u8 = rng.gen();
    let signer = ValidatorSigner::from_int(temp);
    let author = signer.author();

    let vote = Vote::new(
        VoteData::new(BlockInfo::random(round), BlockInfo::random(round - 1)),
        author,
        LedgerInfo::new(BlockInfo::empty(), HashValue::zero()),
        &signer,
    );
    let sync_info = generate_SyncInfo();
    VoteMsg::new(vote,sync_info)
}

pub fn mutate_vote(cur_pro: VoteMsg) -> VoteMsg{
     let mut rng = rand::thread_rng();
     let round : u64 = rng.gen();
     let temp: u8 = rng.gen();

     let vote = cur_pro.vote();
     let signature = vote.signature();
     let propose = vote.vote_data().proposed();
     let parent = vote.vote_data().parent();
     let new_propose = mutate_blockInfo(propose);
     let new_parent = mutate_blockInfo(parent);
     let commit_info = vote.ledger_info().commit_info();
     let new_commit = mutate_blockInfo(commit_info);
     //let consensus_data_hash = vote.ledger_info().consensus_data_hash();
     let vote_data = VoteData::new(new_propose, new_parent);
     let consensus_data_hash = vote_data.hash();

     let author = vote.author();
     let sync_info = cur_pro.sync_info();

     let vote = Vote::new_with_signature(
        vote_data,
        author,
        LedgerInfo::new(new_commit,consensus_data_hash),
        signature.clone()
         );
     VoteMsg::new(vote,sync_info.clone())
}
fn mutate_blockInfo(cur_info: &BlockInfo) -> BlockInfo{
    let next_epoch_state = cur_info.next_epoch_state();
    let mut epoch = cur_info.epoch();
    let mut round = cur_info.round();
    let mut version = cur_info.version();
    let mut timestamp_usecs = cur_info.timestamp_usecs();
    let id = cur_info.id();
    let executed_state_id = cur_info.executed_state_id();
    let mut rng = rand::thread_rng();
    let temp1: u64 = rng.gen();
    if temp1 % 4 == 0{
        epoch = epoch + (temp1 % 10);
    }
    else if temp1 % 4 == 1 && epoch > 10{
        epoch = epoch - (temp1 % 10)
    }
    else if temp1 % 4 == 2{
        epoch = temp1;
    }

    let temp2: u64 = rng.gen();
    if temp2 % 4 == 0{
        round = round + (temp2 % 10);
    }
    else if temp2 % 4 == 1 && round > 10{
        round = round - (temp2 % 10);
    }
    else if temp2 % 4 == 2{
        round = temp2;
    }

    let temp3: u64 = rng.gen();
    if temp3 % 4 == 0{
        version = version + (temp3 % 10);
    }
    else if temp3 % 4 ==1 && version > 10{
        version = version - (temp3 % 10);
    }
    else if temp3 % 4 ==2 {
        version =temp3;
    }

    let temp4: u64 = rng.gen();
    if temp4 % 4 == 0{
        timestamp_usecs += temp4 % 10000;
    }
    else if temp4 % 4 ==1 && timestamp_usecs > 10000{
        timestamp_usecs -= temp4 % 10000;
    }
    else if temp4 % 4 ==2{
        timestamp_usecs = temp4;
    }

    //epoch = temp1;
    //round = temp2;
    //version = temp3;
    //timestamp_usecs = temp4;

    BlockInfo::new(epoch,round,id,executed_state_id,version,timestamp_usecs,next_epoch_state.cloned())
}

fn generate_CommitVoteMsg() -> CommitVote{
    let mut rng = rand::thread_rng();
    let temp : u8 = rng.gen();
    let signer = ValidatorSigner::from_int(temp);
    let author = signer.author();
    let ledger_info = LedgerInfo::mock_genesis(None);
    CommitVote::new(author, ledger_info, &signer)
}

fn generate_CommitDecisionMsg() -> CommitDecision{
    let ledger_info = LedgerInfo::mock_genesis(None);
    let lis = LedgerInfoWithSignatures::new(ledger_info, BTreeMap::new());
    CommitDecision::new(lis)
}




