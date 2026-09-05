package health

type Checker interface {
	IsHealthy() bool
	ReportFailure()
}