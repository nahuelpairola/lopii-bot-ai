package messaging

import (
	"reflect"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/pendingjob"
)

type fakeNudgeRepo struct{}

func (fakeNudgeRepo) SentKeys(uint64) ([]string, error)     { return nil, nil }
func (fakeNudgeRepo) MarkSent(uint64, string) error         { return nil }
func (fakeNudgeRepo) MarkSentAgain(uint64, string) error    { return nil }
func (fakeNudgeRepo) MarkTapped(uint64, string) error       { return nil }
func (fakeNudgeRepo) LastSentAt(uint64) (*time.Time, error) { return nil, nil }

type fakeJobsRepo struct{}

func (fakeJobsRepo) Insert(*pendingjob.PendingJob) error { return nil }
func (fakeJobsRepo) ListByUserOrdered(uint64) ([]pendingjob.PendingJob, error) {
	return nil, nil
}
func (fakeJobsRepo) ListPendingUserIDs() ([]uint64, error) { return nil, nil }
func (fakeJobsRepo) Delete(uint64) error                   { return nil }
func (fakeJobsRepo) CountByUser(uint64) (int64, error)     { return 0, nil }

func TestNewController_WiresEveryDependency(t *testing.T) {
	c := NewController(
		&fakeUserRepository{},
		&fakeInvitationRepository{},
		&fakeAccountRepoFull{},
		&fakeMovementRepoFull{},
		&fakeSubcategoryRepoFull{},
		conversation.NewEngine(&fakeConvStore{}, FlowResumeLabel),
		&fakeFullOrchestrator{},
		&fakeMetricRepo{},
		&stubChatHistory{},
		&fakeReminderRepo{},
		&fakeTraceRepo{},
		fakeNudgeRepo{},
		fakeJobsRepo{},
		&fakeActionsRepo{},
	)

	v := reflect.ValueOf(c).Elem()
	typ := v.Type()
	for i := range typ.NumField() {
		f := typ.Field(i)
		switch v.Field(i).Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func:
			if v.Field(i).IsNil() {
				t.Errorf("NewController dejó %s.%s en nil: falta la línea en el literal de retorno",
					typ.Name(), f.Name)
			}
		default:
		}
	}
}
