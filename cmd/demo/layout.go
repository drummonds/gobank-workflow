package main

// layoutHTML is the page frame: Bulma navbar with the Workflows dropdown,
// then the page content. Hrefs are relative to <base href> so the same
// markup works at "/" on the server and under the service-worker scope.
const layoutHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<base href="{{.base}}">
<title>gobank-workflow demo</title>
<link rel="stylesheet" href="bulma.min.css">
<link rel="icon" href="favicon.ico">
<style>.navbar-item.is-active{font-weight:600} pre.log{max-height:16rem;overflow:auto}</style>
</head>
<body>
<nav class="navbar is-primary" role="navigation" aria-label="main navigation">
  <div class="navbar-brand">
    <a class="navbar-item" href=""><strong>gobank-workflow</strong></a>
    <a role="button" class="navbar-burger" aria-label="menu" aria-expanded="false" data-target="nav">
      <span aria-hidden="true"></span><span aria-hidden="true"></span><span aria-hidden="true"></span><span aria-hidden="true"></span>
    </a>
  </div>
  <div id="nav" class="navbar-menu">
    <div class="navbar-start">
      <div class="navbar-item has-dropdown is-hoverable">
        <a class="navbar-link{{if or (eq .active "instances") (eq .active "definitions")}} is-active{{end}}">Workflows</a>
        <div class="navbar-dropdown">
          <a class="navbar-item{{if eq .active "instances"}} is-active{{end}}" href="workflows/instances">Instances</a>
          <a class="navbar-item{{if eq .active "definitions"}} is-active{{end}}" href="workflows/definitions">Definitions</a>
        </div>
      </div>
      <a class="navbar-item{{if eq .active "simulation"}} is-active{{end}}" href="simulation">Simulation</a>
      <a class="navbar-item{{if eq .active "explorer"}} is-active{{end}}" href="internal/explorer">DB Explorer</a>
      <a class="navbar-item{{if eq .active "about"}} is-active{{end}}" href="about">About</a>
    </div>
    <div class="navbar-end"><span class="navbar-item has-text-grey-light">{{.version}}</span></div>
  </div>
</nav>
<section class="section"><div class="container">
{{.content}}
</div></section>
<script>
document.querySelectorAll('.navbar-burger').forEach(b=>b.addEventListener('click',()=>{b.classList.toggle('is-active');document.getElementById(b.dataset.target).classList.toggle('is-active');}));
</script>
</body>
</html>`
