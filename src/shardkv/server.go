package shardkv


// import "shardmaster"
import (
	"bytes"
	"labrpc"
	"shardmaster"
	"time"
)
import "raft"
import "sync"
import "encoding/gob"

const (
	GET = "GET"
	PUT = "PUT"
	APPEND = "APPEND"
)

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Key       string
	Value     string
	Operation string
	ClientId  int64
	ReqId     int64
}


type ApplyReply struct {
	value string
	err   string
}

type MigrationArg struct {

}

type MigrationReply struct {

}

type ShardKV struct {
	mu           sync.Mutex
	me           int
	rf           *raft.Raft
	applyCh      chan raft.ApplyMsg
	make_end     func(string) *labrpc.ClientEnd
	gid          int
	masters      []*labrpc.ClientEnd
	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	store       map[string]string
	executed    map[int64]int64
	opChans     map[int]chan ApplyReply
	shardMaster *shardmaster.Clerk
	config      shardmaster.Config
}

func (kv *ShardKV) startAgree(op Op) ApplyReply {

	//raft.Log("server.go: server %d startAgree, op: {key: %s, value: %s, op: %s}," +
	//	" clientId %d, reqId: %d\n", kv.me, op.Key, op.Value, op.Operation, op.ClientId, op.ReqId)

	index, term, _ := kv.rf.Start(op)
	kv.mu.Lock()
	opChan, ok := kv.opChans[index]
	if !ok {
		kv.opChans[index] = make(chan ApplyReply, 1)
		opChan= kv.opChans[index]
	}
	kv.mu.Unlock()
	reply := ApplyReply{}
	select {
	case reply = <- opChan:
		curTerm, isLeader := kv.rf.GetState()
		if !isLeader || term != curTerm {
			reply.value = ""
			reply.err = ERROR
		}
	case <-time.After(1000 * time.Millisecond):
		reply.err = ERROR
	}
	//raft.Log("server.go: server %d startAgree over, op: {key: %s, value: %s, op: %s}," +
	//	" clientId %d, reqId: %d, result: %s \n", kv.me, op.Key, op.Value, op.Operation, op.ClientId, op.ReqId, reply.err)
	return reply
}

func (kv *ShardKV) apply(op Op) ApplyReply {
	//raft.Log3("server.go: server %d apply, op: {key: %s, value: %s, op: %s}," +
	//	" clientId %d, reqId: %d\n", kv.me, op.Key, op.Value, op.Operation, op.ClientId, op.ReqId)
	kv.mu.Lock()
	defer kv.mu.Unlock()
	reply := ApplyReply{}
	reply.err = OK
	reply.value = ""
	executedReqId, _ := kv.executed[op.ClientId]
	if op.ReqId > executedReqId {
		if op.Operation == GET {
			v, ok := kv.store[op.Key]
			if ok {
				reply.value = v
			} else {
				reply.err = ErrNoKey
			}
		} else if op.Operation == APPEND {
			v, ok := kv.store[op.Key]
			if ok {
				kv.store[op.Key] = v + op.Value
			} else {
				reply.err = ErrNoKey
			}
		} else {
			kv.store[op.Key] = op.Value
		}
		kv.executed[op.ClientId] = op.ReqId
	} else {
		//raft.Log("server.go: server %d waitToAggre failed, op: {key: %s, value: %s, op: %s}," +
		//	" clientId %d, reqId: %d\n", kv.me, op.Key, op.Value, op.Operation, op.ClientId, op.ReqId)
		if op.Operation == GET {
			reply.value = kv.store[op.Key]
		}
		reply.err = ErrDupReq
	}
	//raft.Log3("server.go: server %d apply over, op: {key: %s, value: %s, op: %s}," +
	//	" clientId %d, reqId: %d, err: %s\n", kv.me, op.Key, op.Value, op.Operation, op.ClientId, op.ReqId, reply.err)
	return reply
}

func (kv *ShardKV) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	_, isLeader := kv.rf.GetState()
	if !isLeader {
		reply.WrongLeader = true
		return
	}

	//raft.Log3("server.go: server %d Get, {key: %s}, isLeader: %t, clientId: %d\n",
	//	kv.me, args.Key, isLeader, args.Id)

	reply.WrongLeader = false

	op := Op{args.Key, "", GET, args.Id, args.ReqId}

	applyReply := kv.startAgree(op)

	reply.Value = applyReply.value
	reply.Err = Err(applyReply.err)

	//raft.Log3("server.go: server %d Get over, {key: %s, value: %s}, isLeader: %t, clientId: %d, err: %s\n",
	//	kv.me, args.Key, reply.Value, isLeader, args.Id, reply.Err)

}

func (kv *ShardKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	_, isLeader := kv.rf.GetState()
	if !isLeader {
		reply.WrongLeader = true
		return
	}
	//raft.Log3("server.go: server %d PutAppend, {key: %s, value: %s, op: %s}, clientId: %d\n",
	//	kv.me, args.Key, args.Value, args.Op, args.Id)
	reply.WrongLeader = false

	op := Op{args.Key, args.Value, args.Op, args.Id, args.ReqId}

	applyReply := kv.startAgree(op)

	reply.Err = Err(applyReply.err)

	//raft.Log3("server.go: server %d PutAppend over, {key: %s, value: %s, op: %s}, leader: %t, clientId: %d, err: %s \n",
	//	kv.me, args.Key, kv.store[op.Key], args.Op, isLeader, args.Id, reply.Err)
}

//
// the tester calls Kill() when a ShardKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *ShardKV) Kill() {
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *ShardKV) Migration(arg MigrationArg, reply MigrationReply) {

}

func (kv *ShardKV) doPoll() {
	_, isLeader := kv.rf.GetState()
	if !isLeader {
		return
	}
	newConfig := kv.shardMaster.Query(-1)

	if newConfig.Num > kv.config.Num {
		kv.rf.Start(newConfig)
	}
}

func (kv *ShardKV) applyNewConfig(newConfig *shardmaster.Config) {

	pushShards := make([]int, 0)
	pullShards := make([]int, 0)


	for shard, gid := range kv.config.Shards {
		if gid == kv.gid {
			if newConfig.Shards[shard] != kv.gid {
				pushShards = append(pushShards, shard)
			}
		} else {
			if newConfig.Shards[shard] == kv.gid {
				pullShards = append(pullShards, shard)
			}
		}
	}

	for _, shard := range pullShards {

		gid := newConfig.Shards[shard]
		migrationArg := MigrationArg{}

		if servers, ok := newConfig.Groups[gid]; ok {
			for si := 0; si < len(servers); si++ {
				srv := kv.make_end(servers[si])

				var reply PutAppendReply
				ok := srv.Call("ShardKV.Migration", &migrationArg, &reply)
				if ok && reply.WrongLeader == false && (reply.Err == OK || reply.Err == ErrDupReq){
					return
				}
				if ok && reply.Err == ErrWrongGroup {
					break
				}
			}
		}
	}
}


//
// servers[] contains the ports of the servers in this group.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots with
// persister.SaveSnapshot(), and Raft should save its state (including
// log) with persister.SaveRaftState().
//
// the k/v server should snapshot when Raft's saved state exceeds
// maxraftstate bytes, in order to allow Raft to garbage-collect its
// log. if maxraftstate is -1, you don't need to snapshot.
//
// gid is this group's GID, for interacting with the shardmaster.
//
// pass masters[] to shardmaster.MakeClerk() so you can send
// RPCs to the shardmaster.
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs. You'll need this to send RPCs to other groups.
//
// look at client.go for examples of how to use masters[]
// and make_end() to send RPCs to the group owning a specific shard.
//
// StartServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int, gid int, masters []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *ShardKV {
	// call gob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	gob.Register(Op{})

	kv := new(ShardKV)
	kv.me = me
	kv.maxraftstate = maxraftstate
	kv.make_end = make_end
	kv.gid = gid
	kv.masters = masters
	kv.shardMaster = shardmaster.MakeClerk(masters)
	kv.config = kv.shardMaster.Query(-1)

	// Your initialization code here.

	// Use something like this to talk to the shardmaster:
	// kv.mck = shardmaster.MakeClerk(kv.masters)

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	kv.opChans = make(map[int]chan ApplyReply)
	kv.store = make(map[string]string)
	kv.executed = make(map[int64]int64)

	raft.Log3("start shardKV server %d, gid: %d \n", gid, me)
	go waitToAgree(kv)
	go poll(kv)

	return kv
}

func poll(kv *ShardKV) {

	for {
		select {
		case <-time.After(100 * time.Millisecond):
			kv.doPoll()
		}
	}
}


func waitToAgree(kv *ShardKV) {

	for {
		applyMsg := <- kv.applyCh

		if applyMsg.UseSnapshot {
			r := bytes.NewBuffer(applyMsg.Snapshot)
			d := gob.NewDecoder(r)

			kv.mu.Lock()
			kv.store = make(map[string]string)
			kv.executed = make(map[int64]int64)
			d.Decode(&kv.executed)
			d.Decode(&kv.store)
			kv.mu.Unlock()

		} else {

			op, ok := applyMsg.Command.(Op)

			if ok {
				_, isLeader := kv.rf.GetState()

				raft.Log("server.go: server %d waitToAgree , op: {key: %s, value: %s, op: %s}, "+
					"isLeader: %t, clientId: %d, reqId: %d\n", kv.me, op.Key, op.Value, op.Operation, isLeader, op.ClientId, op.ReqId)

				reply := kv.apply(op)

				if isLeader {
					kv.mu.Lock()
					op, ok := kv.opChans[applyMsg.Index]
					kv.mu.Unlock()
					if ok {
						op <- reply
					}
				}

				kv.mu.Lock()
				if kv.maxraftstate != -1 && kv.rf.GetRaftStateSize() >= kv.maxraftstate {
					raft.Log2("server.go: server %d, max_raft_state: %d, cur_raft_state: %d \n", kv.me, kv.maxraftstate, kv.rf.GetRaftStateSize())
					w := new(bytes.Buffer)
					e := gob.NewEncoder(w)
					e.Encode(kv.executed)
					e.Encode(kv.store)
					data := w.Bytes()
					go kv.rf.SnapShot(data, applyMsg.Index)
				}
				kv.mu.Unlock()
				raft.Log("server.go: server %d waitToAgree over, op: {key: %s, value: %s, op: %s}, "+
					"isLeader: %t, clientId: %d, reqId: %d\n", kv.me, op.Key, op.Value, op.Operation, isLeader, op.ClientId, op.ReqId)
			} else {
				newConfig := applyMsg.Command.(shardmaster.Config)
				kv.applyNewConfig(&newConfig)
			}
		}
	}

}
