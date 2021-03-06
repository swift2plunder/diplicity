window.GameMemberView = BaseView.extend({

  template: _.template($('#game_member_underscore').html()),

	tagName: 'tr',

	initialize: function(options) {
		this.member = options.member;
	},

  render: function() {
	  var that = this;
    that.$el.html(that.template({
			member: that.member,
		}));
		return that;
	},

});
