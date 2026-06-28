(function() {
    // 获取当前的 script 标签
    var scripts = document.getElementsByTagName('script');
    var currentScript = scripts[scripts.length - 1];
    if (document.currentScript) {
        currentScript = document.currentScript;
    }

    var formId = currentScript.getAttribute('data-id');
    var theme = currentScript.getAttribute('data-theme') || 'light';
    var lang = currentScript.getAttribute('data-lang') || 'en';

    if (!formId) {
        console.error('Feedback Form: Missing data-id attribute.');
        return;
    }

    // 动态获取 embed.js 所在的宿主域名，以便正确拼接 /f/:id
    var scriptSrc = currentScript.src;
    var parser = document.createElement('a');
    parser.href = scriptSrc;
    var baseUrl = parser.protocol + '//' + parser.host;

    // 创建 iframe
    var iframe = document.createElement('iframe');
    var iframeUrl = baseUrl + '/f/' + encodeURIComponent(formId) + 
                    '?theme=' + encodeURIComponent(theme) + 
                    '&lang=' + encodeURIComponent(lang) + 
                    '&source=' + encodeURIComponent(window.location.href);
                    
    iframe.src = iframeUrl;
    iframe.style.width = '100%';
    iframe.style.border = 'none';
    iframe.style.overflow = 'hidden';
    iframe.style.minHeight = '50px'; // 初始最小高度
    iframe.setAttribute('scrolling', 'no');

    // 将 iframe 插入到当前 script 标签的后面
    currentScript.parentNode.insertBefore(iframe, currentScript.nextSibling);

    // 监听来自 iframe 的高度变化消息
    window.addEventListener('message', function(event) {
        // 安全检查：确保消息来源于我们的服务
        if (event.origin !== baseUrl) {
            return;
        }

        try {
            var data = typeof event.data === 'string' ? JSON.parse(event.data) : event.data;
            if (data && data.type === 'feedback-resize' && data.height) {
                // 如果页面上有多个表单，可以通过 formId 区分
                if (data.formId === formId || !data.formId) {
                    iframe.style.height = data.height + 'px';
                }
            }
        } catch (e) {
            // 忽略解析错误
        }
    });
})();
