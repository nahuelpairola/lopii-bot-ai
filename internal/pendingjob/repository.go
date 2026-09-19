package pendingjob

import "lopiibot.com/internal/database"

type Repository interface {
	Insert(job *PendingJob) error
	ListByUserOrdered(userID uint64) ([]PendingJob, error)
	ListPendingUserIDs() ([]uint64, error)
	Delete(id uint64) (bool, error)
	CountByUser(userID uint64) (int64, error)
}

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) Insert(job *PendingJob) error {
	return r.conn.DB.Create(job).Error
}

func (r *repository) ListByUserOrdered(userID uint64) ([]PendingJob, error) {
	var jobs []PendingJob
	err := r.conn.DB.Where("user_id = ?", userID).Order("created_at").Find(&jobs).Error
	return jobs, err
}

func (r *repository) ListPendingUserIDs() ([]uint64, error) {
	var ids []uint64
	err := r.conn.DB.Model(&PendingJob{}).Distinct().Pluck("user_id", &ids).Error
	return ids, err
}

func (r *repository) Delete(id uint64) (bool, error) {
	res := r.conn.DB.Delete(&PendingJob{}, "id = ?", id)
	return res.RowsAffected == 1, res.Error
}

func (r *repository) CountByUser(userID uint64) (int64, error) {
	var n int64
	err := r.conn.DB.Model(&PendingJob{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}
