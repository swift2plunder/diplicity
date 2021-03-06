package game

import (
	"fmt"
	"time"

	"github.com/zond/diplicity/common"
	"github.com/zond/diplicity/epoch"
	"github.com/zond/diplicity/user"
	"github.com/zond/godip/classical"
	"github.com/zond/godip/classical/orders"
	dip "github.com/zond/godip/common"
	"github.com/zond/godip/state"
	"github.com/zond/kcwraps/kol"
)

func ScheduleUnresolvedPhases(c common.SkinnyContext) (err error) {
	unresolved := Phases{}
	if err = c.DB().Query().Where(kol.Equals{"Resolved", false}).All(&unresolved); err != nil {
		return
	}
	for index, _ := range unresolved {
		(&unresolved[index]).Schedule(c)
	}
	return
}

type Phase struct {
	Id     kol.Id
	GameId kol.Id `kol:"index"`

	Season   dip.Season
	Year     int
	Type     dip.PhaseType
	Ordinal  int
	Resolved bool `kol:"index"`

	Units         map[dip.Province]dip.Unit
	Orders        map[dip.Nation]map[dip.Province][]string
	SupplyCenters map[dip.Province]dip.Nation
	Dislodgeds    map[dip.Province]dip.Unit
	Dislodgers    map[dip.Province]dip.Province
	Bounces       map[dip.Province]map[dip.Province]bool
	Resolutions   map[dip.Province]string

	Deadline time.Duration

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (self *Phase) ShortString() string {
	return fmt.Sprintf("%v %v, %v", self.Season, self.Year, self.Type)
}

func (self *Phase) autoResolve(c common.SkinnyContext) (err error) {
	c.Infof("Auto resolving %v/%v due to timeout", self.GameId, self.Id)
	if err = c.Transact(func(c common.SkinnyContext) (err error) {
		if err = c.DB().Get(self); err != nil {
			err = fmt.Errorf("While trying to load %+v: %v", self, err)
			return
		}
		if self.Resolved {
			c.Infof("%+v was already resolved", self)
			return
		}
		game := &Game{Id: self.GameId}
		if err = c.DB().Get(game); err != nil {
			err = fmt.Errorf("While trying to load %+v's game: %v", self, err)
			return
		}
		return game.resolve(c, self)
	}); err != nil {
		return
	}
	return
}

func (self *Phase) Schedule(c common.SkinnyContext) error {
	if !self.Resolved {
		ep, err := epoch.Get(c.DB())
		if err != nil {
			return err
		}
		timeout := self.Deadline - ep
		c.BetweenTransactions(func(c common.SkinnyContext) {
			if timeout > 0 {
				time.AfterFunc(timeout, func() {
					if err := self.autoResolve(c); err != nil {
						c.Errorf("Failed resolving %+v after %v: %v", self, timeout, err)
					}
				})
				c.Debugf("Scheduled resolution of %v/%v in %v at %v", self.GameId, self.Id, timeout, time.Now().Add(timeout))
			} else {
				c.Debugf("Resolving %v/%v immediately, it is %v overdue", self.GameId, self.Id, -timeout)
				if err := self.autoResolve(c); err != nil {
					c.Errorf("Failed resolving %+v immediately: %v", self, err)
				}
			}
		})
	}
	return nil
}

func (self *Phase) emailTo(c common.SkinnyContext, game *Game, member *Member, user *user.User) (err error) {
	to := fmt.Sprintf("%v <%v>", member.Nation, user.Email)
	unsubTag := &common.UnsubscribeTag{
		T: common.UnsubscribePhaseEmail,
		U: user.Id,
	}
	unsubTag.H = unsubTag.Hash(c.Secret())
	encodedUnsubTag, err := unsubTag.Encode()
	if err != nil {
		return
	}
	contextLink, err := user.I("To see this in context: http://%v/games/%v", user.DiplicityHost, self.GameId)
	if err != nil {
		return
	}
	unsubLink, err := user.I("To unsubscribe: http://%v/unsubscribe/%v", user.DiplicityHost, encodedUnsubTag)
	if err != nil {
		return
	}
	text, err := user.I("A new phase has been created")
	if err != nil {
		return
	}
	subject, err := game.Describe(c, user)
	if err != nil {
		return
	}
	body := fmt.Sprintf(common.EmailTemplate, text, contextLink, unsubLink)
	go c.SendMail("diplicity", c.ReceiveAddress(), subject, body, []string{to})
	return
}

func (self *Phase) SendStartedEmails(c common.SkinnyContext, game *Game) (err error) {
	members, err := game.Members(c.DB())
	if err != nil {
		return
	}
	for _, member := range members {
		user := &user.User{Id: member.UserId}
		if err = c.DB().Get(user); err != nil {
			return
		}
		if !user.PhaseEmailDisabled {
			subKey := fmt.Sprintf("/games/%v", game.Id)
			if !c.IsSubscribing(user.Email, subKey, common.SubscriptionTimeout) {
				if err = self.emailTo(c, game, &member, user); err != nil {
					c.Errorf("Failed sending to %#v: %v", user.Id.String(), err)
					return
				}
			} else {
				c.Infof("Not sending to %#v, already subscribing to %#v", user.Email, subKey)
			}
		} else {
			c.Infof("Not sending to %#v, phase email disabled", user.Email)
		}
	}

	return
}

func (self *Phase) Game(d *kol.DB) (result *Game, err error) {
	result = &Game{Id: self.GameId}
	err = d.Get(result)
	return
}

func (self *Phase) Updated(d *kol.DB, old *Phase) {
	g := Game{Id: self.GameId}
	if err := d.Get(&g); err != nil {
		panic(err)
	}
	d.EmitUpdate(&g)
}

func (self *Phase) redact(member *Member) *Phase {
	if self == nil {
		return nil
	}
	result := *self
	if !self.Resolved {
		for nat, _ := range self.Orders {
			if member == nil || member.Nation != nat {
				delete(result.Orders, nat)
			}
		}
	}
	return &result
}

func (self *Phase) Options(nation dip.Nation) (result dip.Options, err error) {
	state, err := self.State()
	if err != nil {
		return
	}
	result = state.Phase().Options(state, nation)
	return
}

func (self *Phase) State() (result *state.State, err error) {
	parsedOrders, err := orders.ParseAll(self.Orders)
	if err != nil {
		return
	}
	units := map[dip.Province]dip.Unit{}
	for prov, unit := range self.Units {
		units[prov] = unit
	}
	orders := map[dip.Nation]map[dip.Province][]string{}
	for nat, ord := range self.Orders {
		orders[nat] = ord
	}
	supplyCenters := map[dip.Province]dip.Nation{}
	for prov, nat := range self.SupplyCenters {
		supplyCenters[prov] = nat
	}
	dislodgeds := map[dip.Province]dip.Unit{}
	for prov, unit := range self.Dislodgeds {
		dislodgeds[prov] = unit
	}
	dislodgers := map[dip.Province]dip.Province{}
	for k, v := range self.Dislodgers {
		dislodgers[k] = v
	}
	bounces := map[dip.Province]map[dip.Province]bool{}
	for prov, b := range self.Bounces {
		bounces[prov] = b
	}
	result = classical.Blank(classical.Phase(
		self.Year,
		self.Season,
		self.Type,
	)).Load(
		units,
		supplyCenters,
		dislodgeds,
		dislodgers,
		bounces,
		parsedOrders,
	)
	return
}

type Phases []Phase

func (self Phases) Len() int {
	return len(self)
}

func (self Phases) Less(j, i int) bool {
	return self[i].Ordinal < self[j].Ordinal
}

func (self Phases) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}
