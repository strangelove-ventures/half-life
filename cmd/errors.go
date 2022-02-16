package cmd

import (
	"fmt"
	"time"
)

type JailedError struct{ msg string }

func (e *JailedError) Error() string { return e.msg }
func newJailedError(until time.Time) *JailedError {
	return &JailedError{fmt.Sprintf("validator is jailed until %s", until.String())}
}

type TombstonedError struct{ msg string }

func (e *TombstonedError) Error() string { return e.msg }
func newTombstonedError() *TombstonedError {
	return &TombstonedError{"validator is tombstoned"}
}

type OutOfSyncError struct{ msg string }

func (e *OutOfSyncError) Error() string { return e.msg }
func newOutOfSyncError(address string) *OutOfSyncError {
	return &OutOfSyncError{fmt.Sprintf("rpc server %s out of sync, cannot get up to date information", address)}
}

type BlockFetchError struct{ msg string }

func (e *BlockFetchError) Error() string { return e.msg }
func newBlockFetchError(height int64, address string) *BlockFetchError {
	return &BlockFetchError{fmt.Sprintf("error fetching block %d from rpc server %s", height, address)}
}

type MissedRecentBlocksError struct{ msg string }

func (e *MissedRecentBlocksError) Error() string { return e.msg }
func newMissedRecentBlocksError(missed int64) *MissedRecentBlocksError {
	return &MissedRecentBlocksError{fmt.Sprintf("missed %d/%d most recent blocks", missed, recentBlocksToCheck)}
}

type GenericRPCError struct{ msg string }

func (e *GenericRPCError) Error() string { return e.msg }
func newGenericRPCError(msg string) *GenericRPCError {
	return &GenericRPCError{msg}
}
