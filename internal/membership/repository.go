package membership

type Repository interface {
	Create(membership Membership) error
	GetByID(id string) (Membership, error)
	GetByUserAndProject(userID, projectID string) (Membership, error)
	ListByProject(projectID string) ([]Membership, error)
	ListByUser(userID string) ([]Membership, error)
	Update(membership Membership) error
	Delete(id string) error
}
