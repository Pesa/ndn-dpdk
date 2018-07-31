#!/bin/bash
if [[ $1 == 'help' ]]; then
  cat <<EOT
Subcommands:
  version [show]
    Show version.
  face [list]
    List faces.
  face show <ID>
    Show face counters.
  face create <REMOTEURI> [<LOCALURI>]
    Create a socket face.
  face destroy <ID>
    Destroy a face.
  ndt show
    Show NDT content.
  ndt counters
    Show NDT counters.
  ndt update [HASH] [VALUE]
    Update an NDT element by hash.
  ndt updaten [NAME] [VALUE]
    Update an NDT element by name.
  strategy [list]
    List forwarding strategies
  strategy load [NAME] [ELF-FILE]
    Load forwarding strategy.
  fib info
    Show FIB counters.
  fib list
    List FIB entry names.
  fib insert <NAME> <NEXTHOP,NEXTHOP>
    Insert/replace FIB entry.
  fib erase <NAME>
    Erase FIB entry.
  fib find <NAME>
  fib lpm <NAME>
    Perform exact-match/longest-prefix-match lookup on FIB.
  fib counters <NAME>
    Show FIB entry counters.
  dpinfo [global]
    Show dataplane global information.
  dpinfo input <I>
  dpinfo fwd <I>
  dpinfo pit <I>
  dpinfo cs <I>
    Show dataplane i-th input/fwd/PIT/CS counters.
EOT
  exit 0
fi

jsonrpc() {
  METHOD=$1
  PARAMS=$2
  if [[ -z $2 ]]; then PARAMS='{}'; fi
  jayson -s 127.0.0.1:6345 -m $METHOD -p "$PARAMS"
}

if [[ $1 == 'version' ]]; then
  if [[ -z $2 ]] || [[ $2 == 'show' ]]; then
    jsonrpc Version.Version
  fi
elif [[ $1 == 'face' ]]; then
  if [[ -z $2 ]] || [[ $2 == 'list' ]]; then
    jsonrpc Face.List
  elif [[ $2 == 'show' ]]; then
    jsonrpc Face.Get '{"Id":'$3'}'
  elif [[ $2 == 'create' ]]; then
    jsonrpc Face.Create '{"RemoteUri":"'$3'","LocalUri":"'$4'"}'
  elif [[ $2 == 'destroy' ]]; then
    jsonrpc Face.Destroy '{"Id":'$3'}'
  fi
elif [[ $1 == 'ndt' ]]; then
  if [[ $2 == 'show' ]]; then
    jsonrpc Ndt.ReadTable ''
  elif [[ $2 == 'counters' ]]; then
    jsonrpc Ndt.ReadCounters ''
  elif [[ $2 == 'update' ]]; then
    jsonrpc Ndt.Update '{"Hash":'$3',"Value":'$4'}'
  elif [[ $2 == 'updaten' ]]; then
    jsonrpc Ndt.Update '{"Name":"'$3'","Value":'$4'}'
  fi
elif [[ $1 == 'strategy' ]]; then
  if [[ -z $2 ]] || [[ $2 == 'list' ]]; then
    jsonrpc Strategy.List ''
  elif [[ $2 == 'show' ]]; then
    jsonrpc Strategy.Get '{"Id":'$3'}'
  elif [[ $2 == 'load' ]]; then
    jsonrpc Strategy.Load '{"Name":"'$3'","Elf":"'$(base64 -w0 $4)'"}'
  elif [[ $2 == 'unload' ]]; then
    jsonrpc Strategy.Unload '{"Id":'$3'}'
  fi
elif [[ $1 == 'fib' ]]; then
  if [[ $2 == 'info' ]]; then
    jsonrpc Fib.Info ''
  elif [[ -z $2 ]] || [[ $2 == 'list' ]]; then
    jsonrpc Fib.List ''
  elif [[ $2 == 'insert' ]]; then
    jsonrpc Fib.Insert '{"Name":"'$3'","Nexthops":['$4']}'
  elif [[ $2 == 'erase' ]] || [[ $2 == 'find' ]] || [[ $2 == 'lpm' ]]; then
    jsonrpc Fib."${2^}" '{"Name":"'$3'"}'
  elif [[ $2 == 'counters' ]]; then
    jsonrpc Fib.ReadEntryCounters '{"Name":"'$3'"}'
  fi
elif [[ $1 == 'dpinfo' ]]; then
  if [[ -z $2 ]] || [[ $2 == 'global' ]]; then
    jsonrpc DpInfo.Global ''
  elif [[ $2 == 'input' ]] || [[ $2 == 'fwd' ]] || [[ $2 == 'pit' ]] || [[ $2 == 'cs' ]]; then
    jsonrpc DpInfo."${2^}" '{"Index":'$3'}'
  fi
fi
