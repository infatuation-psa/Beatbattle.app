const ipc = require('node-ipc');
const {servicePath} = require('config.json');
ipc.config.maxRetries = 3;

ipc.connectTo('service', servicePath, ()=>{
  console.log('Connected to service')
  ipc.of.service.emit('service', process.argv.slice(2));
});
setTimeout(()=>{
  ipc.disconnect('service')
}, 1000)
