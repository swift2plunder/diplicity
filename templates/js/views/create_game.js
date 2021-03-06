window.CreateGameView = BaseView.extend({

  template: _.template($('#create_game_underscore').html()),

	initialize: function(options) {
		this.listenTo(window.session.user, 'change', this.doRender);
		var deadlines = {};
		var chatFlags = {};
		_.each(variantMap[defaultVariant].PhaseTypes, function(type) {
		  deadlines[type] = defaultDeadline;
      chatFlags[type] = defaultChatFlags;
		});
		chatFlags['BeforeGame'] = defaultChatFlags;
		chatFlags['AfterGame'] = defaultChatFlags;
		var member = {
		  UserId: btoa(window.session.user.get('Email')),
			User: {
			  Email: window.session.user.get('Email'),
			},
		};
		this.gameState = new GameState({
		  Members: [member],
			Private: false,
		  Variant: defaultVariant,
			Deadlines: deadlines,
			ChatFlags: chatFlags,
			State: {{.GameState "Created"}},
			AllocationMethod: defaultAllocationMethod,
      NonCommitConsequences: defaultNonCommitConsequences,
			NMRConsequences: defaultNMRConsequences,
			Ranking: true,
		});
		this.gameState.url = '/games';
	},

  render: function() {
		var that = this;
		navLinks(mainButtons);
		that.gameState.get('Members')[0].User = window.session.user.attributes;
		that.$el.html(that.template({
		  user: window.session.user,
		}));
		if (window.session.user.loggedIn()) {
		  var save_call = function() {
				that.gameState.save(null, {
					success: function() {
						navigate('/');
					},
				});
			};
			var state_view = new GameStateView({ 
				parentId: 'create-game',
				editable: true,
				model: that.gameState,
			}).doRender();
			that.$('#create-game').append(state_view.el);
		}
		that.$('#create-game').css('margin-bottom', $('#bottom-navigation').height() + 'px');
		return that;
	},

});
