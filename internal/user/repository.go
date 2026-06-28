package user

type Repository interface {
	Create(user User) error
	GetByID(id string) (User, error)
	GetByUsername(username string) (User, error)
	Update(user User) error
	List() ([]User, error)
}
