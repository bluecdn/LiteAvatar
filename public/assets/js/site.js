(function(){
    fetch('/stats', {cache:'no-store'})
        .then(r => r.json())
        .then(d => {
            var n = d.requests || 0;
            document.getElementById('req-count').textContent =
                n.toLocaleString('zh-CN');
        })
        .catch(() => { document.getElementById('req-count').textContent = '--'; });
})();

// MD5 - Paul Johnston, BSD - https://pajhome.org.uk/crypt/md5/
var md5=function(){function s(s,n){var t=(s&65535)+(n&65535);return(s>>16)+(n>>16)+(t>>16)<<16|t&65535}function n(s,n){return s<<n|s>>>32-n}function t(t,r,o,e,c,u){return s(n(s(s(r,t),s(e,u)),c),o)}function r(s,n,r,o,e,c,u){return t(n&r|~n&o,s,n,e,c,u)}function o(s,n,r,o,e,c,u){return t(n&o|r&~o,s,n,e,c,u)}function e(s,n,r,o,e,c,u){return t(n^r^o,s,n,e,c,u)}function c(s,n,r,o,e,c,u){return t(r^(n|~o),s,n,e,c,u)}function u(n,t){n[t>>5]|=128<<t%32;n[(t+64>>>9<<4)+14]=t;var u,a,f,i,l,h=1732584193,d=-271733879,g=-1732584194,m=271733878;for(u=0;u<n.length;u+=16){a=h;f=d;i=g;l=m;h=r(h,d,g,m,n[u],7,-680876936);m=r(m,h,d,g,n[u+1],12,-389564586);g=r(g,m,h,d,n[u+2],17,606105819);d=r(d,g,m,h,n[u+3],22,-1044525330);h=r(h,d,g,m,n[u+4],7,-176418897);m=r(m,h,d,g,n[u+5],12,1200080426);g=r(g,m,h,d,n[u+6],17,-1473231341);d=r(d,g,m,h,n[u+7],22,-45705983);h=r(h,d,g,m,n[u+8],7,1770035416);m=r(m,h,d,g,n[u+9],12,-1958414417);g=r(g,m,h,d,n[u+10],17,-42063);d=r(d,g,m,h,n[u+11],22,-1990404162);h=r(h,d,g,m,n[u+12],7,1804603682);m=r(m,h,d,g,n[u+13],12,-40341101);g=r(g,m,h,d,n[u+14],17,-1502002290);d=r(d,g,m,h,n[u+15],22,1236535329);h=o(h,d,g,m,n[u+1],5,-165796510);m=o(m,h,d,g,n[u+6],9,-1069501632);g=o(g,m,h,d,n[u+11],14,643717713);d=o(d,g,m,h,n[u],20,-373897302);h=o(h,d,g,m,n[u+5],5,-701558691);m=o(m,h,d,g,n[u+10],9,38016083);g=o(g,m,h,d,n[u+15],14,-660478335);d=o(d,g,m,h,n[u+4],20,-405537848);h=o(h,d,g,m,n[u+9],5,568446438);m=o(m,h,d,g,n[u+14],9,-1019803690);g=o(g,m,h,d,n[u+3],14,-187363961);d=o(d,g,m,h,n[u+8],20,1163531501);h=o(h,d,g,m,n[u+13],5,-1444681467);m=o(m,h,d,g,n[u+2],9,-51403784);g=o(g,m,h,d,n[u+7],14,1735328473);d=o(d,g,m,h,n[u+12],20,-1926607734);h=e(h,d,g,m,n[u+5],4,-378558);m=e(m,h,d,g,n[u+8],11,-2022574463);g=e(g,m,h,d,n[u+11],16,1839030562);d=e(d,g,m,h,n[u+14],23,-35309556);h=e(h,d,g,m,n[u+1],4,-1530992060);m=e(m,h,d,g,n[u+4],11,1272893353);g=e(g,m,h,d,n[u+7],16,-155497632);d=e(d,g,m,h,n[u+10],23,-1094730640);h=e(h,d,g,m,n[u+13],4,681279174);m=e(m,h,d,g,n[u],11,-358537222);g=e(g,m,h,d,n[u+3],16,-722521979);d=e(d,g,m,h,n[u+6],23,76029189);h=e(h,d,g,m,n[u+9],4,-640364487);m=e(m,h,d,g,n[u+12],11,-421815835);g=e(g,m,h,d,n[u+15],16,530742520);d=e(d,g,m,h,n[u+2],23,-995338651);h=c(h,d,g,m,n[u],6,-198630844);m=c(m,h,d,g,n[u+7],10,1126891415);g=c(g,m,h,d,n[u+14],15,-1416354905);d=c(d,g,m,h,n[u+5],21,-57434055);h=c(h,d,g,m,n[u+12],6,1700485571);m=c(m,h,d,g,n[u+3],10,-1894986606);g=c(g,m,h,d,n[u+10],15,-1051523);d=c(d,g,m,h,n[u+1],21,-2054922799);h=c(h,d,g,m,n[u+8],6,1873313359);m=c(m,h,d,g,n[u+15],10,-30611744);g=c(g,m,h,d,n[u+6],15,-1560198380);d=c(d,g,m,h,n[u+13],21,1309151649);h=c(h,d,g,m,n[u+4],6,-145523070);m=c(m,h,d,g,n[u+11],10,-1120210379);g=c(g,m,h,d,n[u+2],15,718787259);d=c(d,g,m,h,n[u+9],21,-343485551);h=s(h,a);d=s(d,f);g=s(g,i);m=s(m,l)}return[h,d,g,m]}function a(s){var n,t=[],r=(1<<8)-1;for(n=0;n<s.length*8;n+=8)t[n>>5]|=(s.charCodeAt(n/8)&r)<<n%32;return t}function f(s){var n,t,r="";for(t=0;t<32;t+=8)r+=(n=s[t>>5]>>>t%32&255).toString(16).padStart(2,"0");return r}function i(s){return unescape(encodeURIComponent(s))}return function(s){s=i(s);return f([].concat.apply([],u(a(s),s.length*8).map((s)=>{var n=[];for(var t=0;t<32;t+=8)n.push(s>>>t&255);return n})).reduce((s,n,t)=>{s[t>>2]=(s[t>>2]||0)|n<<t%4*8;return s},[]))}}();
// 简化的纯净版（运行验证）：md5("") === "d41d8cd98f00b204e9800998ecf8427e"
md5=function(){var hex_chr="0123456789abcdef".split("");function rotateLeft(x,n){return(x<<n)|(x>>>(32-n))}function addUnsigned(x,y){var x4=(x&0x40000000),y4=(y&0x40000000),x8=(x&0x80000000),y8=(y&0x80000000),lsw=(x&0x3FFFFFFF)+(y&0x3FFFFFFF);if(x4&y4){return(lsw^0x80000000^x8^y8)}if(x4|y4){if(lsw&0x40000000){return(lsw^0xC0000000^x8^y8)}else{return(lsw^0x40000000^x8^y8)}}else{return(lsw^x8^y8)}}function F(x,y,z){return(x&y)|((~x)&z)}function G(x,y,z){return(x&z)|(y&(~z))}function H(x,y,z){return(x^y^z)}function I(x,y,z){return(y^(x|(~z)))}function FF(a,b,c,d,x,s,ac){a=addUnsigned(a,addUnsigned(addUnsigned(F(b,c,d),x),ac));return addUnsigned(rotateLeft(a,s),b)}function GG(a,b,c,d,x,s,ac){a=addUnsigned(a,addUnsigned(addUnsigned(G(b,c,d),x),ac));return addUnsigned(rotateLeft(a,s),b)}function HH(a,b,c,d,x,s,ac){a=addUnsigned(a,addUnsigned(addUnsigned(H(b,c,d),x),ac));return addUnsigned(rotateLeft(a,s),b)}function II(a,b,c,d,x,s,ac){a=addUnsigned(a,addUnsigned(addUnsigned(I(b,c,d),x),ac));return addUnsigned(rotateLeft(a,s),b)}function convertToWordArray(string){var lWordCount,lMessageLength=string.length,lNumberOfWords_temp1=lMessageLength+8,lNumberOfWords_temp2=(lNumberOfWords_temp1-(lNumberOfWords_temp1%64))/64,lNumberOfWords=(lNumberOfWords_temp2+1)*16,lWordArray=Array(lNumberOfWords-1),lBytePosition=0,lByteCount=0;while(lByteCount<lMessageLength){lWordCount=(lByteCount-(lByteCount%4))/4;lBytePosition=(lByteCount%4)*8;lWordArray[lWordCount]=(lWordArray[lWordCount]|(string.charCodeAt(lByteCount)<<lBytePosition));lByteCount++}lWordCount=(lByteCount-(lByteCount%4))/4;lBytePosition=(lByteCount%4)*8;lWordArray[lWordCount]=lWordArray[lWordCount]|(0x80<<lBytePosition);lWordArray[lNumberOfWords-2]=lMessageLength<<3;lWordArray[lNumberOfWords-1]=lMessageLength>>>29;return lWordArray}function wordToHex(lValue){var WordToHexValue="",WordToHexValue_temp="",lByte,lCount;for(lCount=0;lCount<=3;lCount++){lByte=(lValue>>>(lCount*8))&255;WordToHexValue_temp="0"+lByte.toString(16);WordToHexValue=WordToHexValue+WordToHexValue_temp.substr(WordToHexValue_temp.length-2,2)}return WordToHexValue}function utf8Encode(string){string=string.replace(/\r\n/g,"\n");var utftext="";for(var n=0;n<string.length;n++){var c=string.charCodeAt(n);if(c<128){utftext+=String.fromCharCode(c)}else if((c>127)&&(c<2048)){utftext+=String.fromCharCode((c>>6)|192);utftext+=String.fromCharCode((c&63)|128)}else{utftext+=String.fromCharCode((c>>12)|224);utftext+=String.fromCharCode(((c>>6)&63)|128);utftext+=String.fromCharCode((c&63)|128)}}return utftext}return function(string){var x=Array();var k,AA,BB,CC,DD,a,b,c,d;var S11=7,S12=12,S13=17,S14=22;var S21=5,S22=9,S23=14,S24=20;var S31=4,S32=11,S33=16,S34=23;var S41=6,S42=10,S43=15,S44=21;string=utf8Encode(string);x=convertToWordArray(string);a=0x67452301;b=0xEFCDAB89;c=0x98BADCFE;d=0x10325476;for(k=0;k<x.length;k+=16){AA=a;BB=b;CC=c;DD=d;a=FF(a,b,c,d,x[k+0],S11,0xD76AA478);d=FF(d,a,b,c,x[k+1],S12,0xE8C7B756);c=FF(c,d,a,b,x[k+2],S13,0x242070DB);b=FF(b,c,d,a,x[k+3],S14,0xC1BDCEEE);a=FF(a,b,c,d,x[k+4],S11,0xF57C0FAF);d=FF(d,a,b,c,x[k+5],S12,0x4787C62A);c=FF(c,d,a,b,x[k+6],S13,0xA8304613);b=FF(b,c,d,a,x[k+7],S14,0xFD469501);a=FF(a,b,c,d,x[k+8],S11,0x698098D8);d=FF(d,a,b,c,x[k+9],S12,0x8B44F7AF);c=FF(c,d,a,b,x[k+10],S13,0xFFFF5BB1);b=FF(b,c,d,a,x[k+11],S14,0x895CD7BE);a=FF(a,b,c,d,x[k+12],S11,0x6B901122);d=FF(d,a,b,c,x[k+13],S12,0xFD987193);c=FF(c,d,a,b,x[k+14],S13,0xA679438E);b=FF(b,c,d,a,x[k+15],S14,0x49B40821);a=GG(a,b,c,d,x[k+1],S21,0xF61E2562);d=GG(d,a,b,c,x[k+6],S22,0xC040B340);c=GG(c,d,a,b,x[k+11],S23,0x265E5A51);b=GG(b,c,d,a,x[k+0],S24,0xE9B6C7AA);a=GG(a,b,c,d,x[k+5],S21,0xD62F105D);d=GG(d,a,b,c,x[k+10],S22,0x2441453);c=GG(c,d,a,b,x[k+15],S23,0xD8A1E681);b=GG(b,c,d,a,x[k+4],S24,0xE7D3FBC8);a=GG(a,b,c,d,x[k+9],S21,0x21E1CDE6);d=GG(d,a,b,c,x[k+14],S22,0xC33707D6);c=GG(c,d,a,b,x[k+3],S23,0xF4D50D87);b=GG(b,c,d,a,x[k+8],S24,0x455A14ED);a=GG(a,b,c,d,x[k+13],S21,0xA9E3E905);d=GG(d,a,b,c,x[k+2],S22,0xFCEFA3F8);c=GG(c,d,a,b,x[k+7],S23,0x676F02D9);b=GG(b,c,d,a,x[k+12],S24,0x8D2A4C8A);a=HH(a,b,c,d,x[k+5],S31,0xFFFA3942);d=HH(d,a,b,c,x[k+8],S32,0x8771F681);c=HH(c,d,a,b,x[k+11],S33,0x6D9D6122);b=HH(b,c,d,a,x[k+14],S34,0xFDE5380C);a=HH(a,b,c,d,x[k+1],S31,0xA4BEEA44);d=HH(d,a,b,c,x[k+4],S32,0x4BDECFA9);c=HH(c,d,a,b,x[k+7],S33,0xF6BB4B60);b=HH(b,c,d,a,x[k+10],S34,0xBEBFBC70);a=HH(a,b,c,d,x[k+13],S31,0x289B7EC6);d=HH(d,a,b,c,x[k+0],S32,0xEAA127FA);c=HH(c,d,a,b,x[k+3],S33,0xD4EF3085);b=HH(b,c,d,a,x[k+6],S34,0x4881D05);a=HH(a,b,c,d,x[k+9],S31,0xD9D4D039);d=HH(d,a,b,c,x[k+12],S32,0xE6DB99E5);c=HH(c,d,a,b,x[k+15],S33,0x1FA27CF8);b=HH(b,c,d,a,x[k+2],S34,0xC4AC5665);a=II(a,b,c,d,x[k+0],S41,0xF4292244);d=II(d,a,b,c,x[k+7],S42,0x432AFF97);c=II(c,d,a,b,x[k+14],S43,0xAB9423A7);b=II(b,c,d,a,x[k+5],S44,0xFC93A039);a=II(a,b,c,d,x[k+12],S41,0x655B59C3);d=II(d,a,b,c,x[k+3],S42,0x8F0CCC92);c=II(c,d,a,b,x[k+10],S43,0xFFEFF47D);b=II(b,c,d,a,x[k+1],S44,0x85845DD1);a=II(a,b,c,d,x[k+8],S41,0x6FA87E4F);d=II(d,a,b,c,x[k+15],S42,0xFE2CE6E0);c=II(c,d,a,b,x[k+6],S43,0xA3014314);b=II(b,c,d,a,x[k+13],S44,0x4E0811A1);a=II(a,b,c,d,x[k+4],S41,0xF7537E82);d=II(d,a,b,c,x[k+11],S42,0xBD3AF235);c=II(c,d,a,b,x[k+2],S43,0x2AD7D2BB);b=II(b,c,d,a,x[k+9],S44,0xEB86D391);a=addUnsigned(a,AA);b=addUnsigned(b,BB);c=addUnsigned(c,CC);d=addUnsigned(d,DD)}return(wordToHex(a)+wordToHex(b)+wordToHex(c)+wordToHex(d)).toLowerCase()}}();

// 复制按钮
document.querySelectorAll('.code-copy-btn').forEach(btn => {
    btn.addEventListener('click', () => {
        const text = btn.getAttribute('data-copy') || btn.parentElement.querySelector('pre').innerText;
        navigator.clipboard.writeText(text).then(() => {
            btn.classList.add('copied');
            const icon = btn.querySelector('i');
            const old = icon.className;
            icon.className = 'fa-solid fa-check';
            setTimeout(() => { icon.className = old; btn.classList.remove('copied'); }, 1200);
        });
    });
});

// Demo 三种 tab
const tabs = document.querySelectorAll('.demo-tab');
const inp = document.getElementById('demo-input');
const iconEl = document.getElementById('demo-icon');
const sel = document.getElementById('demo-size');
const img = document.getElementById('demo-img');
const urlA = document.getElementById('demo-url');
const md5Row = document.getElementById('demo-md5-row');
const md5El = document.getElementById('demo-md5');
const sourceBadge = document.getElementById('demo-source');
const cacheEl = document.getElementById('demo-cache');

const TAB_CONFIG = {
    email: { placeholder: '输入邮箱地址 (例: someone@example.com)', icon: 'fa-envelope',     example: 'someone@gmail.com' },
    hash:  { placeholder: '输入 32-64 位 MD5 hex (邮箱已 MD5 后)', icon: 'fa-fingerprint', example: 'd5d30d232682e6176045145b20befc5c' },
    qq:    { placeholder: '输入 QQ 号 (5-12 位数字)',                  icon: 'fa-brands fa-qq', example: '10000' },
};
let currentTab = 'email';

tabs.forEach(t => t.addEventListener('click', () => {
    tabs.forEach(x => x.classList.remove('active'));
    t.classList.add('active');
    currentTab = t.dataset.tab;
    const cfg = TAB_CONFIG[currentTab];
    inp.placeholder = cfg.placeholder;
    iconEl.className = 'icon-prefix fa-solid ' + (cfg.icon.includes('brands') ? cfg.icon : 'fa-solid ' + cfg.icon);
    iconEl.classList.add('fa-solid');
    inp.value = cfg.example;
    md5Row.classList.toggle('is-hidden', currentTab !== 'email');
    update();
}));

// 初始化
inp.placeholder = TAB_CONFIG.email.placeholder;
iconEl.className = 'icon-prefix fa-solid fa-envelope';
inp.value = TAB_CONFIG.email.example;

let updateTimer;
function update() {
    let val = (inp.value || '').trim();
    if (!val) return;
    let id = val;
    if (currentTab === 'email') {
        id = md5(val.toLowerCase());
        md5El.textContent = id;
        md5Row.classList.remove('is-hidden');
    } else {
        md5Row.classList.add('is-hidden');
    }
    let url = 'https://gravatar.bluecdn.com/avatar/' + encodeURIComponent(id);
    if (sel.value) url += '?s=' + sel.value;
    // QQ 邮箱回退：纯数字 QQ 号 @qq.com → 后端无注册头像时自动回退 QQ 头像
    if (currentTab === 'email') {
        const qqMatch = val.trim().match(/^(\d{5,12})@qq\.com$/i);
        if (qqMatch) {
            const sep0 = url.includes('?') ? '&' : '?';
            url += sep0 + 'qq=' + qqMatch[1];
        }
    }
    urlA.textContent = url;
    urlA.href = url;
    // demo 用 refresh=1 强制后端探测，避开任何 cache 异常
    const sep = url.includes('?') ? '&' : '?';
    img.src = url + sep + 'refresh=1&_t=' + Date.now();
    img.onerror = () => { setTimeout(() => { img.src = url + sep + 'refresh=1&_t=' + Date.now(); }, 300); };
    sourceBadge.innerHTML = '<i class="fa-solid fa-circle-notch fa-spin"></i>探测中';
    sourceBadge.classList.remove('is-default');
    cacheEl.textContent = '…';
    fetch(url, { method: 'HEAD', cache: 'no-store' }).then(r => {
        const src = r.headers.get('x-avatar-source') || 'unknown';
        const cache = r.headers.get('x-cache-status') || '-';
        const cc = r.headers.get('cache-control') || '';
        if (src.endsWith('-default')) {
            sourceBadge.classList.add('is-default');
            sourceBadge.innerHTML = '<i class="fa-solid fa-circle-question"></i>' + src;
        } else {
            sourceBadge.innerHTML = '<i class="fa-solid fa-circle-check"></i>' + src;
        }
        const m = cc.match(/max-age=(\d+)/);
        const ttl = m ? (parseInt(m[1]) >= 86400 ? Math.round(parseInt(m[1])/86400) + ' 天' : Math.round(parseInt(m[1])/60) + ' 分钟') : '-';
        cacheEl.textContent = `${cache} (TTL ${ttl})`;
    }).catch(() => {
        sourceBadge.innerHTML = '<i class="fa-solid fa-triangle-exclamation"></i>请求失败';
        cacheEl.textContent = '-';
    });
}

inp.addEventListener('input', () => {
    clearTimeout(updateTimer);
    updateTimer = setTimeout(update, 300);
});
sel.addEventListener('change', update);
update();
