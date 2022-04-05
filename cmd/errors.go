package cmd

import (
	"fmt"
	"math"
	"time"
)

type JailedError struct{ until time.Time }

func (e *JailedError) Error() string {
	return fmt.Sprintf("validator is jailed until %s", e.until.String())
}
func newJailedError(until time.Time) *JailedError {
	return &JailedError{until}
}

type TombstonedError struct{}

func (e *TombstonedError) Error() string { return "validator is tombstoned" }
func newTombstonedError() *TombstonedError {
	return &TombstonedError{}
}

type OutOfSyncError struct{ msg string }

func (e *OutOfSyncError) Error() string { return e.msg }
func newOutOfSyncError(address string) *OutOfSyncError {
	return &OutOfSyncError{fmt.Sprintf("rpc server %s out of sync, cannot get up to date information", address)}
}

type ChainHaltError struct {
	durationNano int64
}

func (e *ChainHaltError) Error() string {
	minutesHalted := int64(math.Round(float64(e.durationNano) / 6e10))
	return fmt.Sprintf("rpc node has been halted for %dmin", minutesHalted)
}
func newChainHaltError(durationNano int64) *ChainHaltError {
	return &ChainHaltError{durationNano: durationNano}
}

type BlockFetchError struct {
	height  int64
	address string
}

func (e *BlockFetchError) Error() string {
	return fmt.Sprintf("error fetching block %d from rpc server %s", e.height, e.address)
}
func newBlockFetchError(height int64, address string) *BlockFetchError {
	return &BlockFetchError{height, address}
}

type MissedRecentBlocksError struct{ missed int64 }

func (e *MissedRecentBlocksError) Error() string {
	return fmt.Sprintf("missed %d/%d most recent blocks", e.missed, recentBlocksToCheck)
}
func newMissedRecentBlocksError(missed int64) *MissedRecentBlocksError {
	return &MissedRecentBlocksError{missed}
}

type GenericRPCError struct{ msg string }

func (e *GenericRPCError) Error() string { return e.msg }
func newGenericRPCError(msg string) *GenericRPCError {
	return &GenericRPCError{msg}
}

type SentryGRPCError struct {
	sentry string
	msg    string
}

func (e *SentryGRPCError) Error() string { return fmt.Sprintf("%s - %s", e.sentry, e.msg) }
func newSentryGRPCError(sentry string, msg string) *SentryGRPCError {
	return &SentryGRPCError{sentry, msg}
}

type SentryOutOfSyncError struct {
	sentry string
	msg    string
}

func (e *SentryOutOfSyncError) Error() string { return fmt.Sprintf("%s - %s", e.sentry, e.msg) }
func newSentryOutOfSyncError(sentry string, msg string) *SentryOutOfSyncError {
	return &SentryOutOfSyncError{sentry, msg}
}

type SentryHaltError struct {
	sentry       string
	durationNano int64
}

func (e *SentryHaltError) Error() string {
	minutesHalted := int64(math.Round(float64(e.durationNano) / 6e10))
	return fmt.Sprintf("%s has been halted for %dmin", e.sentry, minutesHalted)
}
func newSentryHaltError(sentry string, durationNano int64) *SentryHaltError {
	return &SentryHaltError{sentry, durationNano}
}
