(() => {
  document.documentElement.classList.add('js');

  const header = document.querySelector('[data-header]');
  const navToggle = document.querySelector('[data-nav-toggle]');
  const nav = document.getElementById('site-nav');

  const onScroll = () => {
    header?.classList.toggle('is-scrolled', window.scrollY > 8);
  };
  onScroll();
  window.addEventListener('scroll', onScroll, { passive: true });

  const setNav = (open) => {
    if (!nav || !navToggle) return;
    nav.classList.toggle('is-open', open);
    navToggle.setAttribute('aria-expanded', String(open));
    navToggle.setAttribute('aria-label', open ? '关闭菜单' : '打开菜单');
  };

  navToggle?.addEventListener('click', () => {
    setNav(!nav?.classList.contains('is-open'));
  });
  nav?.addEventListener('click', (e) => {
    if (e.target.closest('a')) setNav(false);
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') setNav(false);
  });

  const tabs = [...document.querySelectorAll('[data-shot-tab]')];
  const panels = [...document.querySelectorAll('[data-shot-panel]')];
  const selectShot = (name) => {
    tabs.forEach((tab) => {
      const on = tab.dataset.shotTab === name;
      tab.setAttribute('aria-selected', String(on));
      tab.tabIndex = on ? 0 : -1;
    });
    panels.forEach((panel) => {
      const on = panel.dataset.shotPanel === name;
      panel.hidden = !on;
      panel.classList.toggle('is-active', on);
    });
  };
  tabs.forEach((tab, i) => {
    tab.addEventListener('click', () => selectShot(tab.dataset.shotTab));
    tab.addEventListener('keydown', (e) => {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
      e.preventDefault();
      const step = e.key === 'ArrowRight' ? 1 : -1;
      const next = tabs[(i + step + tabs.length) % tabs.length];
      next.focus();
      selectShot(next.dataset.shotTab);
    });
  });

  document.querySelectorAll('[data-reveal-group]').forEach((group) => {
    [...group.children].forEach((el, i) => {
      if (el.hasAttribute('data-reveal')) {
        el.style.setProperty('--reveal-delay', `${Math.min(i * 70, 350)}ms`);
      }
    });
  });

  const reveals = [...document.querySelectorAll('[data-reveal]')];
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  if (reveals.length) {
    if (reduceMotion.matches || !('IntersectionObserver' in window)) {
      reveals.forEach((el) => el.classList.add('is-visible'));
    } else {
      const io = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
          if (!entry.isIntersecting) return;
          entry.target.classList.add('is-visible');
          io.unobserve(entry.target);
        });
      }, { rootMargin: '0px 0px -8% 0px', threshold: 0.05 });
      reveals.forEach((el) => io.observe(el));
    }
  }

  document.querySelectorAll('[data-year]').forEach((el) => {
    el.textContent = String(new Date().getFullYear());
  });
})();
