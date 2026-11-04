package aiimport

import "sync"

// userImportLocks contains a *sync.Mutex per user id.
//
// The check for duplicates, the creation of the transactions and the finish of the inbox item must not run at the same time for the same user,
// otherwise two simultaneous imports of the same statement would both find no duplicates and create the same transactions twice.
// The lock is held in memory by the process, so it only works with a single instance of the server (the deployment of this fork).
// Running several instances against the same database is NOT supported by the AI import tools.
var userImportLocks sync.Map

// lockUser waits until the user has no other import running and returns the function which releases the lock
func lockUser(uid int64) func() {
	value, _ := userImportLocks.LoadOrStore(uid, &sync.Mutex{})
	mutex := value.(*sync.Mutex)

	mutex.Lock()

	return mutex.Unlock
}
