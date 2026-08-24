/**
 * TROT.CO.JP MAIN UI INTERACTION SCRIPT
 */

document.addEventListener('DOMContentLoaded', () => {
  const navbar = document.querySelector('.navbar');

  // Navbar Blur & Background effect on Scroll
  window.addEventListener('scroll', () => {
    if (window.scrollY > 50) {
      navbar.style.background = 'rgba(9, 13, 22, 0.9)';
      navbar.style.boxShadow = '0 10px 30px rgba(0, 0, 0, 0.5)';
    } else {
      navbar.style.background = 'rgba(9, 13, 22, 0.7)';
      navbar.style.boxShadow = 'none';
    }
  });

  // Smooth Scroll for Nav Links
  document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', function(e) {
      const targetId = this.getAttribute('href');
      if (targetId === '#' || !targetId) return;

      const targetElem = document.querySelector(targetId);
      if (targetElem) {
        e.preventDefault();
        const headerOffset = 80;
        const elementPosition = targetElem.getBoundingClientRect().top;
        const offsetPosition = elementPosition + window.pageYOffset - headerOffset;

        window.scrollTo({
          top: offsetPosition,
          behavior: 'smooth'
        });
      }
    });
  });
});
