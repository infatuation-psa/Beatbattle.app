window.addEventListener('load',()=>{
  const observer = lozad();
  observer.observe();
  
  $('.battle-rules p').children('a').attr('target', function() {
    if(this.host == location.host) return '_self'
    else return '_blank'
  });
})