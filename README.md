# Beatbattle.app

A web application built for beat battle competitions.

To-do -
Abstract away the ability to grab users to it's own function perhaps?

JULY 23
To-do
Finish javascript cleanup
Add deleting groups.
Battle.Host should be a user object so we can have expanded functionality.
Database actions should return the value as well as an error in order to be idiomatic.
Consistently used stuff (ie: Ads, Alert Messages) should be put into a type so it's easier to maintain.
<div class="battle-information"> should be extrapolated into a template for maintainability purposes.

How-To Setup .env
DISCORD_KEY=
DISCORD_SECRET=
DISCORD_CALLBACK=

REDDIT_KEY=
REDDIT_SECRET=
REDDIT_CALLBACK=
REDDIT_STATE=

MYSQL_USER=
MYSQL_PASS=
MYSQL_DB=

SECURE_KEY64=
SECURE_KEY32=

How-To Setup Config.json
{
  "prefix": "",
  "embedColor": "#",
  "servicePath": "",
  "token": "",
  "url": ""
}


SEO Shit -
https://support.google.com/webmasters/answer/34441?hl=en

PERF TAG = MIGHT MAKE PERFORMANCE WORSE
EFFI TAG - CAN MAKE MORE FFICIENT   

		// VoteColour & LikeColour are workarounds to the limits of ZingGrid.
		// ZingGrid usage should be deprecated eventually.