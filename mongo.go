// Copyright (c) 2013 Jason McVetta.  This is Free Software, released under the
// terms of the GPL v3.  See http://www.gnu.org/copyleft/gpl.html for details.
// Resist intellectual serfdom - the ownership of ideas is akin to slavery.

package btoken

import (
	"code.google.com/p/go-uuid/uuid"
	"labix.org/v2/mgo"
	"time"
)

// NewMongoAuthServer configures a MongoDB-based AuthServer.  If expireAfter is
// not nil, authorizations will be automatically expired.
func NewMongoServer(db *mgo.Database) (Server, error) {
	m := mongoServer{
		db:   db,
		name: "authorizations",
	}
	_, err := m.ExpireAfter(DefaultExpireAfter)
	return &m, err
}

type mongoServer struct {
	db          *mgo.Database
	name        string // Collection name
	expireAfter time.Duration
}

func (m *mongoServer) ensureIndexes() error {
	//
	// Declare Indexes
	//
	idxs := []mgo.Index{
		mgo.Index{
			Key:      []string{"token"},
			Unique:   true,
			DropDups: false,
		},
		mgo.Index{
			Key:      []string{"expiration"},
			Unique:   true,
			DropDups: false,
		},
	}
	c := m.col()
	for _, i := range idxs {
		err := c.EnsureIndex(i)
		if err != nil {
			return err
		}
	}
	return nil
}

// col returns a Collection object in a new mgo session
func (s *mongoServer) col() *mgo.Collection {
	session := s.db.Session.Copy()
	d := session.DB(s.db.Name)
	return d.C(s.name)
}

func (s *mongoServer) ExpireAfter(duration string) (time.Duration, error) {
	if duration == "" {
		return s.expireAfter, nil
	}
	dur, err := time.ParseDuration("8h")
	if err != nil {
		return dur, err
	}
	s.expireAfter = dur
	err = s.ensureIndexes()
	return dur, err

}

func (s *mongoServer) IssueToken(req AuthRequest) (string, error) {
	c := s.col()
	tok := uuid.NewUUID().String()
	scopes := map[string]bool{}
	dur := req.Duration
	if dur.Seconds() == 0 || dur.Nanoseconds() > s.expireAfter.Nanoseconds() {
		dur = s.expireAfter
	}
	exp := time.Now().Add(dur)
	for _, s := range req.Scopes {
		scopes[s] = true
	}
	a := Authorization{
		Token:      tok,
		User:       req.User,
		Scopes:     scopes,
		Expiration: exp,
	}
	err := c.Insert(a)
	return tok, err
}

func (s *mongoServer) GetAuthorization(token string) (Authorization, error) {
	a := Authorization{}
	c := s.col()
	query := struct {
		Token string
	}{
		Token: token,
	}
	q := c.Find(query)
	cnt, err := q.Count()
	if err != nil {
		return a, err
	}
	if cnt < 1 {
		return a, ErrInvalidToken
	}
	err = q.One(&a)
	if err != nil {
		return a, err
	}
	if time.Now().After(a.Expiration) {
		c.Remove(query)
		return a, ErrInvalidToken
	}
	return a, nil
}

// CheckAuth answers whether the holder of a token is a given user who is
// authorized to access a given scope.  If scope is an empty string, scope is
// not checked.
func (s *mongoServer) CheckAuth(token, user, scope string) (bool, error) {
	a, err := s.GetAuthorization(token)
	if err != nil {
		return false, err
	}
	if a.User != user {
		return false, nil
	}
	if scope != "" {
		_, ok := a.Scopes[scope]
		if !ok {
			return false, nil
		}
	}
	return true, nil
}