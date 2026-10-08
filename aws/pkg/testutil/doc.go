// Package testutil provides test doubles for the AWS clients.
//
// It lives in the aws module rather than the core one because a mock has to
// implement the real interface, and that interface drags in the AWS SDK: kept
// in the core, every consumer resolved the SDK just to get a logger mock.
package testutil
