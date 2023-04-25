package models

import (
	"fmt"
	"math"
	"time"
)

type IgnorableError interface {
	error
	AlertActive
}

type AlertActive interface {
	Active(config AlertConfig) bool
}

type IgnoreableError struct{ err error }

func (e *IgnoreableError) Error() string {
	return e.err.Error()
}

func (e *IgnoreableError) Active(_ AlertConfig) bool {
	return true
}

func NewIgnorableError(err error) *IgnoreableError {
	return &IgnoreableError{err}
}

type JailedError struct{ until time.Time }

func (e *JailedError) Error() string {
	return fmt.Sprintf("validator is jailed until %s", e.until.String())
}
func (e *JailedError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeJailed)
}
func NewJailedError(until time.Time) *JailedError {
	return &JailedError{until}
}

type TombstonedError struct{}

func (e *TombstonedError) Error() string { return "validator is tombstoned" }
func (e *TombstonedError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeTombstoned)
}
func NewTombstonedError() *TombstonedError {
	return &TombstonedError{}
}

type OutOfSyncError struct{ msg string }

func (e *OutOfSyncError) Error() string { return e.msg }
func (e *OutOfSyncError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeOutOfSync)
}
func NewOutOfSyncError(address string) *OutOfSyncError {
	return &OutOfSyncError{fmt.Sprintf("rpc server %s out of sync, cannot get up to date information", address)}
}

type ChainHaltError struct {
	durationNano int64
}

func (e *ChainHaltError) Error() string {
	minutesHalted := int64(math.Round(float64(e.durationNano) / 6e10))
	return fmt.Sprintf("rpc node has been halted for %dmin", minutesHalted)
}
func (e *ChainHaltError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeHalt)
}
func NewChainHaltError(durationNano int64) *ChainHaltError {
	return &ChainHaltError{durationNano: durationNano}
}

type BlockFetchError struct {
	height  int64
	address string
}

func (e *BlockFetchError) Error() string {
	return fmt.Sprintf("error fetching block %d from rpc server %s", e.height, e.address)
}
func (e *BlockFetchError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeBlockFetch)
}
func NewBlockFetchError(height int64, address string) *BlockFetchError {
	return &BlockFetchError{height, address}
}

type MissedRecentBlocksError struct {
	missed  int64
	toCheck int64
}

func (e *MissedRecentBlocksError) Error() string {
	return fmt.Sprintf("missed %d/%d most recent blocks", e.missed, e.toCheck)
}
func (e *MissedRecentBlocksError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeMissedRecentBlocks)
}
func NewMissedRecentBlocksError(missed, toCheck int64) *MissedRecentBlocksError {
	return &MissedRecentBlocksError{missed, toCheck}
}

type SlashingSLAError struct {
	uptime float64
	sla    float64
}

func (e *SlashingSLAError) Error() string {
	return fmt.Sprintf("block signing uptime (%.02f%%) under SLA (%.02f%%)", e.uptime, e.sla)
}
func (e *SlashingSLAError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeSlashingSLA)
}
func NewSlashingSLAError(uptime, sla float64) *SlashingSLAError {
	return &SlashingSLAError{uptime, sla}
}

type GenericRPCError struct{ msg string }

func (e *GenericRPCError) Error() string { return e.msg }
func (e *GenericRPCError) Active(config AlertConfig) bool {
	return config.AlertActive(AlertTypeGenericRPC)
}
func NewGenericRPCError(msg string) *GenericRPCError {
	return &GenericRPCError{msg}
}

type SentryGRPCError struct {
	sentry string
	msg    string
}

func (e *SentryGRPCError) Sentry() string {
	return e.sentry
}

func (e *SentryGRPCError) Error() string { return fmt.Sprintf("%s - %s", e.sentry, e.msg) }
func NewSentryGRPCError(sentry string, msg string) *SentryGRPCError {
	return &SentryGRPCError{sentry, msg}
}

type SentryOutOfSyncError struct {
	sentry string
	msg    string
}

func (e *SentryOutOfSyncError) Sentry() string {
	return e.sentry
}

func (e *SentryOutOfSyncError) Error() string { return fmt.Sprintf("%s - %s", e.sentry, e.msg) }
func NewSentryOutOfSyncError(sentry string, msg string) *SentryOutOfSyncError {
	return &SentryOutOfSyncError{sentry, msg}
}

type SentryHaltError struct {
	sentry       string
	durationNano int64
}

func (e *SentryHaltError) Sentry() string {
	return e.sentry
}

func (e *SentryHaltError) Error() string {
	minutesHalted := int64(math.Round(float64(e.durationNano) / 6e10))
	return fmt.Sprintf("%s has been halted for %dmin", e.sentry, minutesHalted)
}
func NewSentryHaltError(sentry string, durationNano int64) *SentryHaltError {
	return &SentryHaltError{sentry, durationNano}
}
