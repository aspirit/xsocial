package model

// JobLease is the lock that makes a scheduled job run on one worker replica per
// interval: ID is the job name, Holder the replica that owns the lease and
// ExpiresAt (unix ms) when it lapses. A replica may take a lease that is expired
// or already its own; everyone else is refused until it expires.
type JobLease struct {
	ID        string `bson:"_id" json:"id"`
	Holder    string `bson:"holder" json:"holder"`
	ExpiresAt int64  `bson:"expires_at" json:"expiresAt"`
}
