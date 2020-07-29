# BattleBot

## Introduction:
   Battlebot is a Discord bot specifically designed to interface with the website [beatbattle.app](http://beatbattle.app). With this bot, users can search for currently running battles / check their status, the server can notify users of deadlines via Direct Message (to be implemented), and much more in the future.

## How to add BattleBot to your server

  For now, you can use [this link](https://discordapp.com/oauth2/authorize?client_id=723292014455554088&scope=bot&permissions=523328), but this will be updated once battlebot is connected to the golang server.

## Commands and services

  The two ways one can interface with the discord bot are through command and services. The difference between these two are while users on a server can run commands, services can only be run on the machine running the bot.

### Commands Continued

  Commands are triggered when a user on a server with the battlebot types in the specified prefix (While the default prefix is "!bbot", this can be changed in the [config.json](https://github.com/SeenTheShadow/BattleBot/blob/master/config.json) file under "prefix"), the command they want to run (i.e. search to search for a specific battle by title), and then arguments after it (i.e. your search query). For example, if you wanted to search for all battles that include the word "Kenny" in them, you would type:
  > !bbot search Kenny

### Services Continued

  The way that services are handled in this bot is through socket communication. Before the bot logs into discord, it will first set up a local server. When a client connects to a server and emit the service event, followed by a service you want to execute, and then the necessary data, the server will interpret this event and respond by running that specific service.

  There are two main ways that you can interface via services:
  1. Running service.js with command line arguments
  2. Sending data through a Unix/Windows socket

  The first way is mainly used for either debugging, development, or manual execution of services. To run services this way, you need to:

  * Open up a terminal of your choice (Make sure node.js is installed)
  * Navigate to the BattleBot directory
  * Run the service with this format:
  > node service.js (service name) (data to pass to service)

  Please note that the data needed for a specific service depends on the service you are running. To test if the socket is set up correctly, you can type in:
  >node service.js test

  After running this, the window running the discord bot should say: *Socket set up correctly!*

  The second way to run services is by emitting the event yourself in your specified language. The socket is being run on an offline Unix/Windows protocol and completely bypasses the network card. While the default path for the socket file is */tmp/bbot.service*, this can be changed in [config.json](https://github.com/SeenTheShadow/BattleBot/blob/master/config.json). After connecting to the server via socket, you can send commands by emitting an event with the name 'service' with the packet sending the name of the service and the data needed to run the service seperated by spaces. Using the nodejs implementation with the node-ipc library found in the [service.js](https://github.com/SeenTheShadow/BattleBot/blob/master/service.js) file as reference, the code should look like this:

  ```javascript
  //requires node-ipc library
  //install with npm install node-ipc if in different  directory than BattleBot
  const ipc = require('node-ipc');
  //you can set servicePath to the file path as a string without the destructuring
  const {servicePath} = require('config.json');
  //connects to service with servicePath, and on connection, will run a function
  ipc.connectTo('service', servicePath, ()=>{
    console.log('Connected to service')
    //of course you would want to replace SERVICE NAME and DATA FOR SERVICE
    ipc.of.service.emit('service', 'SERVICE NAME' + 'DATA FOR SERVICE');
  });
  //This just disconnects the client from the server after 1 second
  setTimeout(()=>{
    ipc.disconnect('service')
  }, 1000)
  ```

  While not tested, this is probably similar in other languages.

## Contact Info

  - Email: seentheshadow@gmail.com
  - Twitter: [@seentheshadow](http://www.twitter.com/SeenTheShadow)
  - Instagram: [@seentheshadow](http://www.instagram.com/SeenTheShadow)
