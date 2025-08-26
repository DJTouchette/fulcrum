const HandlerRegistry = require('./lib/HandlerRegistry');
const HandlerService = require('./lib/HandlerService');
const FulcrumJS = require('./lib/FulcrumJS');

module.exports = {
  HandlerRegistry,
  HandlerService,
  FulcrumJS,
  
  // Factory function for easy setup
  createHandlerService: (options = {}) => {
    return new FulcrumJS(options);
  },
  
  // CLI function for command line usage
  startCLI: (args) => {
    const cli = require('./bin/fulcrum-js');
    return cli.run(args);
  }
};
