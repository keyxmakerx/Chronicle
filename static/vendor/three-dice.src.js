// three.js with the post-processing add-ons the dice use, bundled as one ES module: the add-ons
// import three by its bare name, which a browser can't resolve, and they must share its one copy.
// Rebuild with `make dice-vendor`.
export * from 'three';
export { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js';
export { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js';
export { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';
export { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js';
export { HorizontalBlurShader } from 'three/examples/jsm/shaders/HorizontalBlurShader.js';
export { VerticalBlurShader } from 'three/examples/jsm/shaders/VerticalBlurShader.js';
