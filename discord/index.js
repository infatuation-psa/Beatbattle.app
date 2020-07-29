//init Discord.js
const Discord = require('discord.js');
const client = new Discord.Client();
//require all non discord js node modules needed.
const ipc = require('node-ipc');
const mysql = require('mysql2');
const path = require('path');
require('dotenv').config({
  path: '../.env'
});
//destructuring config
const {
  prefix,
  embedColor,
  servicePath,
  token,
  url
} = require("./config.json");
//universal command object. key is name of command in discord. (Ex. if method is named "battles", user would type in !bbot battles)
const Commands = {
  "battles": {
    "function": function(msg, args) {
      con.query("SELECT * FROM challenges WHERE status='entry' OR status='voting'", function(err, result) {
        if (err) msg.reply(err);
        var battleEmbed = new Discord.MessageEmbed()
          .setColor(embedColor)
          .setTitle('beatbattle.app Battles')
          .setURL(url);
        result.forEach(challenge => {
          //replace method is to get rid of nasty unicode that discord can't use
          if (challenge.status == "entry") {
            battleEmbed.addField(challenge.title.replace(String.fromCharCode(38, 35, 51, 57, 59), String.fromCharCode(39)), "Open - " + timeFormat(challenge.deadline), true);
          } else {
            battleEmbed.addField(challenge.title.replace(String.fromCharCode(38, 35, 51, 57, 59), String.fromCharCode(39)), "Voting - " + timeFormat(challenge.voting_deadline), true);
          }
        });
        //sends embed to user
        msg.reply(battleEmbed);
      })
    },
    "description": "Returns all open battles",
    "example": prefix + "battles"
  },
  "faq": {
    "function": function(msg, args) {
      var faqEmbed = new Discord.MessageEmbed()
        .setColor(embedColor)
        .setTitle('FAQ for beatbattle.app')
        .setURL(url + "/faq");
      msg.reply(faqEmbed);
    },
    "description": "Replies with a link to the FAQ page on the beatbattle website",
    "example": prefix + "faq"
  },
  "help": {
    "function": function(msg, args) {
      if (args.length <= 1) {
        //regular help command with no specific command
        var helpEmbed = new Discord.MessageEmbed()
          .setColor(embedColor)
          .setTitle('List of Available Commands')
          .setURL(url);
        for (let [key, value] of Object.entries(Commands)) {
          helpEmbed.addField(key, value.description, true);
        }
        msg.reply(helpEmbed);
      } else {
        //specific command case
        let argQuery = args.slice(1).toString().toLowerCase();
        if (typeof Commands[argQuery] == 'object') {
          //valid command
          var helpEmbed = new Discord.MessageEmbed()
            .setColor(embedColor)
            .setTitle(argQuery)
            .setURL(url);
          helpEmbed.addField(Commands[argQuery].description, "Example: " + Commands[argQuery].example, true);
          msg.reply(helpEmbed);
        } else {
          //invalid command
          msg.reply("Command not found: " + argQuery)
        }
      }
    },
    "description": "Returns either a list of commands available or a description of a specific command",
    "example": prefix + "help OR " + prefix + " help <command>"
  },
  "search": {
    "function": function(msg, args) {
      con.query("SELECT * FROM challenges WHERE status='entry' OR status='voting'", function(err, result) {
        var s = args.slice(1).join(" ");
        result = result.filter(challenge => challenge.title.toLowerCase().includes(s.toLowerCase()));
        if (result.length == 0) {
          msg.reply("No results found for search term: " + s);
        } else {
          //creates embed with cData
          var battleEmbed = new Discord.MessageEmbed()
            .setColor(embedColor)
            .setTitle('Search Results for: ' + s)
            .setURL(url);
          result.forEach(challenge => {
            //replace method is to get rid of nasty unicode that discord can't use
            if (challenge.status == "entry") {
              battleEmbed.addField(challenge.title.replace(String.fromCharCode(38, 35, 51, 57, 59), String.fromCharCode(39)), "Open - " + timeFormat(challenge.deadline), true);
            } else {
              battleEmbed.addField(challenge.title.replace(String.fromCharCode(38, 35, 51, 57, 59), String.fromCharCode(39)), "Voting - " + timeFormat(challenge.voting_deadline), true);
            }
          });
          //sends embed to user
          msg.reply(battleEmbed);
        }
      });
    },
    "description": "Returns all open battles based on search query",
    "example": prefix + "search <search query>"
  },
  "status": {
    "function": function(msg, args) {
      con.query("SELECT * FROM challenges WHERE status='entry' OR status='voting'", function(err, result) {
        var s = args.slice(1).join(" ");
        result = result.filter(challenge => challenge.title.toLowerCase().includes(s.toLowerCase()));
        switch (result.length) {
          case 0:
            //no results
            msg.reply("No results found for search term: " + s);
            break;
          case 1:
            if (result[0].status.includes("Open")) {
              //one result and Open
              msg.reply("Battle " + result[0].title + " is closing submissions in: " + timeFormat(result[0].deadline));
            } else {
              //one result and voting
              msg.reply("Battle " + result[0].title + " is closing voting in: " + timeFormat(result[0].voting_deadline));
            }
            break;
          default:
            //case of multiple search results
            var battleEmbed = new Discord.MessageEmbed()
              .setColor(embedColor)
              .setTitle('Found multiple battles for search term: ' + s)
              .setURL(url);
            result.forEach(challenge => {
              if (challenge.status == "entry") {
                battleEmbed.addField(challenge.title.replace(String.fromCharCode(38, 35, 51, 57, 59), String.fromCharCode(39)), "Open - " + timeFormat(challenge.deadline), true);
              } else {
                battleEmbed.addField(challenge.title.replace(String.fromCharCode(38, 35, 51, 57, 59), String.fromCharCode(39)), "Voting - " + timeFormat(challenge.voting_deadline), true);
              }
            });
            //sends embed to user
            msg.reply(battleEmbed);
            break;
        }
      });
    },
    "description": "Returns the status of a specific challenge or challenges based on search query",
    "example": prefix + "status <search query>"
  },
};
//universal services object. Same idea as commands, but serverside
const Services = {
  'test': () => {
    console.log('Socket set up correctly!');
  },
  'hourLeft': () => {
    //bot.users.cache.get(id).send("yes")
  }
};
//other random useful functions
function timeFormat(deadline) {
  let now = new Date();
  let msdiff = deadline.getTime() - now.getTime();
  return Math.floor(msdiff / 86400000) + "d " + Math.floor(msdiff / 3600000 % 24) + "h " + Math.ceil(msdiff / 60000 % 60) + "m"
}
//console logs when Bot is logged in
client.on('ready', () => {
  console.log(`Logged in as ${client.user.tag}!`);
});
//if any message is sent in dc guild that starts with prefix, itll check for method in Commands object and if found, run said method.
client.on('message', msg => {
  if (msg.content == prefix) {
    Commands.help.function(msg, []);
  } else if (msg.content.startsWith(prefix)) {

    let m = msg.content.split(" ");
    let args = m.slice(1);
    let c = args[0].toLowerCase();

    console.log("command submitted");
    if (typeof Commands[c] == 'object') {
      Commands[c].function(msg, args);
    } else {
      msg.reply("Command Does not Exist!");
    }
  }
});
//Interprocess Communication with service.js or external socket initialized and connected here
ipc.config.silent = true;
ipc.serve(servicePath);
ipc.server.start();
//upon recieving a message from a client, it will run the appropriate service if it exists.
ipc.server.on('service', (cla) => {
  if (typeof Services[cla[0]] == 'function') {
    Services[cla[0]](cla.slice(1));
  } else {
    console.log(`invalid service called: ${cla[0]}`)
  }
});
//mysql set up
var con = mysql.createConnection({
  host: "localhost",
  port: "3306",
  user: process.env.MYSQL_USER,
  password: process.env.MYSQL_PASS,
  database: process.env.MYSQL_DB,
});
//connect to mysql database
con.connect(function(err) {
  if (err) throw err;
  console.log("Connected to MySQL database");
});
//login command
client.login(token);
