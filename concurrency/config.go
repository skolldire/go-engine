package concurrency

import "time"

type Admission string

const (
	AdmissionBlock  Admission = "block"
	AdmissionReject Admission = "reject"
)

type Ordering string

const (
	OrderingUnordered Ordering = "unordered"
	OrderingPerKey    Ordering = "per_key"
)

type KeyFailurePolicy string

const (
	KeyFailureContinue      KeyFailurePolicy = "continue"
	KeyFailureCancelPending KeyFailurePolicy = "cancel_pending"
)

type ShutdownMode string

const (
	ShutdownDrain  ShutdownMode = "drain"
	ShutdownCancel ShutdownMode = "cancel"
)

type State string

const (
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCanceled  State = "canceled"
)

type PoolConfig struct {
	Workers           int              `json:"workers" yaml:"workers" mapstructure:"workers"`
	QueueCapacity     int              `json:"queue_capacity" yaml:"queue_capacity" mapstructure:"queue_capacity"`
	Admission         Admission        `json:"admission" yaml:"admission" mapstructure:"admission"`
	AdmissionTimeout  time.Duration    `json:"admission_timeout" yaml:"admission_timeout" mapstructure:"admission_timeout"`
	TaskTimeout       time.Duration    `json:"task_timeout" yaml:"task_timeout" mapstructure:"task_timeout"`
	Ordering          Ordering         `json:"ordering" yaml:"ordering" mapstructure:"ordering"`
	MaxActiveKeys     int              `json:"max_active_keys" yaml:"max_active_keys" mapstructure:"max_active_keys"`
	PerKeyCapacity    int              `json:"per_key_capacity" yaml:"per_key_capacity" mapstructure:"per_key_capacity"`
	KeyFailure        KeyFailurePolicy `json:"key_failure" yaml:"key_failure" mapstructure:"key_failure"`
	MaxJobWeight      int64            `json:"max_job_weight" yaml:"max_job_weight" mapstructure:"max_job_weight"`
	MaxInFlightWeight int64            `json:"max_in_flight_weight" yaml:"max_in_flight_weight" mapstructure:"max_in_flight_weight"`
}

func DefaultPoolConfig() PoolConfig {
	return PoolConfig{Workers: 1, Admission: AdmissionBlock, Ordering: OrderingUnordered, MaxJobWeight: 1, MaxInFlightWeight: 1}
}

func (c PoolConfig) Validate() error {
	switch {
	case c.Workers <= 0:
		return invalid("workers", "must be positive")
	case c.QueueCapacity < 0:
		return invalid("queue_capacity", "must be nonnegative")
	case c.Admission != AdmissionBlock && c.Admission != AdmissionReject:
		return invalid("admission", "must be block or reject")
	case c.AdmissionTimeout < 0:
		return invalid("admission_timeout", "must be nonnegative")
	case c.TaskTimeout < 0:
		return invalid("task_timeout", "must be nonnegative")
	case c.MaxJobWeight <= 0:
		return invalid("max_job_weight", "must be positive")
	case c.MaxInFlightWeight < c.MaxJobWeight:
		return invalid("max_in_flight_weight", "must cover max_job_weight")
	}
	switch c.Ordering {
	case OrderingUnordered:
		if c.MaxActiveKeys != 0 || c.PerKeyCapacity != 0 || c.KeyFailure != "" {
			return invalid("ordering", "unordered forbids key settings")
		}
	case OrderingPerKey:
		if c.MaxActiveKeys <= 0 {
			return invalid("max_active_keys", "must be positive")
		}
		if c.PerKeyCapacity <= 0 {
			return invalid("per_key_capacity", "must be positive")
		}
		if c.KeyFailure != KeyFailureContinue && c.KeyFailure != KeyFailureCancelPending {
			return invalid("key_failure", "must be continue or cancel_pending")
		}
	default:
		return invalid("ordering", "must be unordered or per_key")
	}
	return nil
}
