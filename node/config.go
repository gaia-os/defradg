// Copyright 2022 Democratized Data Foundation
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

/* Node configuration, in which NodeOpt functions are applied on Options. */

package node

import (
	"time"

	cconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	ma "github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"
)

// Options is the node options.
type Options struct {
	ListenAddrs       []ma.Multiaddr
	TCPAddr           ma.Multiaddr
	DataPath          string
	EnablePubSub      bool
	EnableRelay       bool
	GRPCServerOptions []grpc.ServerOption
	GRPCDialOptions   []grpc.DialOption
	ConnManager       cconnmgr.ConnManager
}

type NodeOpt func(*Options) error

// NewMergedOptions obtains Options by applying given NodeOpts.
func NewMergedOptions(opts ...NodeOpt) (*Options, error) {
	var options Options
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&options); err != nil {
			return nil, err
		}
	}
	return &options, nil
}

// NewConnManager gives a new ConnManager.
func NewConnManager(low int, high int, grace time.Duration) (cconnmgr.ConnManager, error) {
	c, err := connmgr.NewConnManager(low, high, connmgr.WithGracePeriod(grace))
	if err != nil {
		return nil, err
	}
	return c, nil
}

// DataPath sets the data path.
func DataPath(path string) NodeOpt {
	return func(opt *Options) error {
		opt.DataPath = path
		return nil
	}
}

// WithPubSub enables the pubsub feature.
func WithPubSub(enable bool) NodeOpt {
	return func(opt *Options) error {
		opt.EnablePubSub = enable
		return nil
	}
}

// WithEnableRelay enables the relay feature.
func WithEnableRelay(enable bool) NodeOpt {
	return func(opt *Options) error {
		opt.EnableRelay = enable
		return nil
	}
}

// ListenP2PAddrStrings sets the address to listen on given as strings.
func ListenP2PAddrStrings(addrs ...string) NodeOpt {
	return func(opt *Options) error {
		for _, addrstr := range addrs {
			a, err := ma.NewMultiaddr(addrstr)
			if err != nil {
				return err
			}
			opt.ListenAddrs = append(opt.ListenAddrs, a)
		}
		return nil
	}
}

// ListenTCPAddrString sets the TCP address to listen on, as Multiaddr.
func ListenTCPAddrString(addr string) NodeOpt {
	return func(opt *Options) error {
		a, err := ma.NewMultiaddr(addr)
		if err != nil {
			return err
		}
		opt.TCPAddr = a
		return nil
	}
}

// ListenAddrs sets the address to listen on given as MultiAddr(s).
func ListenAddrs(addrs ...ma.Multiaddr) NodeOpt {
	return func(opt *Options) error {
		opt.ListenAddrs = addrs
		return nil
	}
}
