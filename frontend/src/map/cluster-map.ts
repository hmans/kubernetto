import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { BokehPass } from "three/examples/jsm/postprocessing/BokehPass.js";
import { EffectComposer } from "three/examples/jsm/postprocessing/EffectComposer.js";
import { OutputPass } from "three/examples/jsm/postprocessing/OutputPass.js";
import { RenderPass } from "three/examples/jsm/postprocessing/RenderPass.js";
import { ShaderPass } from "three/examples/jsm/postprocessing/ShaderPass.js";
import { UnrealBloomPass } from "three/examples/jsm/postprocessing/UnrealBloomPass.js";
import { mergeGeometries } from "three/examples/jsm/utils/BufferGeometryUtils.js";

import { buildMapLayout } from "./layout";
import { buildTopologyTree, defaultTopologyTreeLimit } from "./tree";
import type { ClusterMapData, MapItemType, MapLayoutItem } from "./types";

const controllers = new Map<HTMLElement, ClusterMapController>();
const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)");
const clickSelectionMaxDistance = 6;
const dustParticleCount = 520;
type MapDebugWindow = Window & {
  __kubernettoMapDebug?: () => unknown;
};
type MapConnectorRelation = "traffic";
type FocusTween = {
  start: number;
  duration: number;
  fromCamera: THREE.Vector3;
  toCamera: THREE.Vector3;
  fromTarget: THREE.Vector3;
  toTarget: THREE.Vector3;
};
type InstancedPrimitiveType = "clusterResource" | "node" | "pod" | "service" | "warning";
type InstancedCollection = {
  mesh: THREE.InstancedMesh;
  geometry: THREE.BufferGeometry;
  nextIndex: number;
  warningSeeds?: THREE.InstancedBufferAttribute;
};
type InstancedInstance = {
  mesh: THREE.InstancedMesh;
  index: number;
  position: THREE.Vector3;
  baseScale: THREE.Vector3;
  baseColor: THREE.Color;
};

const instancedProxyMaterial = new THREE.MeshBasicMaterial({ visible: false });
const identityQuaternion = new THREE.Quaternion();
const instancedMatrix = new THREE.Matrix4();
const vignetteShader = {
  uniforms: {
    tDiffuse: { value: null },
    darkness: { value: 0.48 },
    offset: { value: 1.12 },
  },
  vertexShader: `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
    }
  `,
  fragmentShader: `
    uniform sampler2D tDiffuse;
    uniform float darkness;
    uniform float offset;
    varying vec2 vUv;
    void main() {
      vec4 color = texture2D(tDiffuse, vUv);
      vec2 centered = (vUv - vec2(0.5)) * offset;
      float vignette = smoothstep(0.18, 0.82, dot(centered, centered));
      color.rgb *= 1.0 - vignette * darkness;
      gl_FragColor = color;
    }
  `,
};
const trafficBeamShader = {
  uniforms: {
    beamTime: { value: 0 },
  },
  vertexShader: `
    uniform float beamTime;
    attribute float beamU;
    attribute float beamSeed;
    attribute float beamBranch;
    attribute float beamEdge;
    attribute float beamWidth;
    attribute vec3 beamSideVector;
    attribute vec3 beamUpVector;
    attribute vec2 beamRadial;
    attribute float beamFacet;
    varying float vU;
    varying float vSeed;
    varying float vBranch;
    varying float vEdge;
    varying float vOriginField;
    varying float vFacet;
    void main() {
      vU = beamU;
      vSeed = beamSeed;
      vBranch = beamBranch;
      vEdge = beamEdge;
      vFacet = beamFacet;
      float originField = 1.0 - smoothstep(0.04, 0.36, beamU);
      vOriginField = originField;
      float driftA = sin(beamU * 19.0 - beamTime * (2.8 + beamSeed * 1.7) + beamSeed * 43.0);
      float driftB = sin(beamU * 31.0 - beamTime * (4.1 + beamSeed * 1.3) + beamSeed * 91.0);
      float snap = sin(beamU * 27.0 - floor(beamTime * 9.0 + beamSeed * 13.0) * 1.618);
      float gatherPulse = 0.55 + 0.45 * sin(beamTime * (5.2 + beamSeed * 2.1) + beamSeed * 31.0);
      float captureOrbit = beamU * 26.0 - beamTime * (4.4 + beamSeed * 2.8) + beamSeed * 6.28318530718;
      float focusedWidth = beamWidth * (1.0 + originField * (1.35 + gatherPulse * 0.9));
      vec3 captureField =
        beamSideVector * cos(captureOrbit) * beamWidth * originField * (1.15 + gatherPulse * 0.82) +
        beamUpVector * sin(captureOrbit * 1.31) * beamWidth * originField * (0.86 + gatherPulse * 0.64);
      vec3 volume =
        beamSideVector * beamRadial.x * focusedWidth +
        beamUpVector * beamRadial.y * focusedWidth;
      vec3 wriggle =
        volume +
        beamSideVector * (driftA * beamWidth * (0.5 + originField * 0.46) + snap * beamWidth * 0.18) +
        beamUpVector * (driftB * beamWidth * (0.3 + originField * 0.32)) +
        captureField;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position + wriggle, 1.0);
    }
  `,
  fragmentShader: `
    uniform float beamTime;
    varying float vU;
    varying float vSeed;
    varying float vBranch;
    varying float vEdge;
    varying float vOriginField;
    varying float vFacet;

    float hash(vec2 p) {
      return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453123);
    }

    float noise(vec2 p) {
      vec2 i = floor(p);
      vec2 f = fract(p);
      vec2 u = f * f * (3.0 - 2.0 * f);
      return mix(
        mix(hash(i + vec2(0.0, 0.0)), hash(i + vec2(1.0, 0.0)), u.x),
        mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x),
        u.y
      );
    }

    void main() {
      float t = beamTime;
      float travel = fract(vU - t * (0.46 + vSeed * 0.16) + vSeed);
      float packet = smoothstep(0.00, 0.08, travel) * (1.0 - smoothstep(0.14, 0.28, travel));
      float coarse = noise(vec2(vU * 12.0 - t * 1.1, vSeed * 19.0));
      float fine = noise(vec2(vU * 58.0 - t * 3.4, vSeed * 23.0));
      float arc = abs(sin(vU * 84.0 + coarse * 8.0 - t * (6.0 + vSeed * 2.2)));
      float snap = smoothstep(0.9, 1.0, arc + fine * 0.18);
      float broken = smoothstep(0.16, 0.30, noise(vec2(vU * 34.0 - t * 2.0, vSeed * 31.0)));
      float branchPulse = mix(1.0, smoothstep(0.72, 1.0, sin(t * (7.0 + vSeed * 3.0) + vSeed * 29.0) * 0.5 + 0.5), vBranch);
      float edgeAlpha = 1.0 - smoothstep(0.58, 0.9, abs(vEdge));
      float sourcePulse = smoothstep(0.58, 1.0, sin(t * (6.2 + vSeed * 1.8) + vSeed * 37.0) * 0.5 + 0.5);
      float sourceGather = vOriginField * (0.08 + sourcePulse * 0.18) * edgeAlpha;
      float facet = mix(0.7, 1.12, vFacet);
      float alpha = (0.18 + snap * 0.36 + packet * 0.66 + sourceGather) * broken * branchPulse * mix(edgeAlpha, 0.62, vBranch) * facet;
      vec3 base = mix(vec3(0.08, 0.92, 0.76), vec3(0.48, 0.92, 1.0), fine);
      vec3 hot = vec3(0.9, 1.0, 0.78);
      vec3 color = mix(base * facet, hot, clamp(packet * 0.68 + snap * 0.22 + sourceGather * 0.66, 0.0, 1.0));
      gl_FragColor = vec4(color, alpha);
    }
  `,
};
const trafficSparkShader = {
  uniforms: {
    sparkTime: { value: 0 },
    sparkOpacity: { value: 0.94 },
  },
  vertexShader: `
    uniform float sparkTime;
    attribute vec3 sparkFrom;
    attribute vec3 sparkControl;
    attribute vec3 sparkTo;
    attribute vec3 sparkSideVector;
    attribute vec3 sparkUpVector;
    attribute float sparkSeed;
    attribute float sparkOffset;
    attribute float sparkSpeed;
    attribute float sparkRadius;
    attribute float sparkSize;
    varying float vSparkAlpha;
    varying float vSparkHot;

    vec3 quadraticPoint(float t) {
      float inv = 1.0 - t;
      return inv * inv * sparkFrom + 2.0 * inv * t * sparkControl + t * t * sparkTo;
    }

    void main() {
      float travel = fract(sparkOffset + sparkTime * sparkSpeed);
      float eased = travel < 0.5 ? 2.0 * travel * travel : 1.0 - pow(-2.0 * travel + 2.0, 2.0) * 0.5;
      float phase = sparkSeed * 6.28318530718;
      float orbit = sparkTime * (1.45 + sparkSeed * 1.2) + eased * 18.0 + phase;
      float chatter = sin(sparkTime * (3.4 + sparkSeed * 1.6) + phase * 3.1) * 0.05;
      float radius = sparkRadius * (0.64 + 0.22 * sin(orbit * 0.73 + phase) + chatter);
      vec3 swirl = sparkSideVector * cos(orbit) * radius + sparkUpVector * sin(orbit + phase * 0.31) * radius * 0.72;
      vec3 point = quadraticPoint(eased) + swirl;
      vec4 mvPosition = modelViewMatrix * vec4(point, 1.0);
      float distanceToCamera = max(1.0, -mvPosition.z);
      float pulse = 0.64 + 0.36 * sin(sparkTime * (3.6 + sparkSeed * 2.6) + phase + eased * 10.0);
      float wrapFade = smoothstep(0.0, 0.06, travel) * (1.0 - smoothstep(0.94, 1.0, travel));
      vSparkAlpha = wrapFade * mix(0.46, 0.9, pulse);
      vSparkHot = smoothstep(0.72, 1.0, pulse);
      gl_PointSize = clamp(sparkSize * (110.0 / distanceToCamera) * mix(0.9, 1.16, pulse), 1.0, 6.2);
      gl_Position = projectionMatrix * mvPosition;
    }
  `,
  fragmentShader: `
    uniform float sparkOpacity;
    varying float vSparkAlpha;
    varying float vSparkHot;
    void main() {
      vec2 point = gl_PointCoord - vec2(0.5);
      float radius = length(point) * 2.0;
      float disc = 1.0 - smoothstep(0.2, 1.0, radius);
      float core = 1.0 - smoothstep(0.0, 0.28, radius);
      float glint = smoothstep(0.82, 0.0, abs(point.x * 0.34 + point.y));
      vec3 cool = vec3(0.12, 0.95, 0.86);
      vec3 hot = vec3(0.98, 1.0, 0.82);
      vec3 color = mix(cool, hot, clamp(core * 0.82 + glint * 0.32 + vSparkHot, 0.0, 1.0));
      float alpha = (disc * 0.58 + core * 0.86 + glint * 0.22) * vSparkAlpha * sparkOpacity;
      gl_FragColor = vec4(color, alpha);
    }
  `,
};
const dustShader = {
  uniforms: {
    dustTime: { value: 0 },
  },
  vertexShader: `
    uniform float dustTime;
    attribute float dustSeed;
    attribute float dustSize;
    varying float vDustAlpha;
    varying float vDustBlur;
    void main() {
      float phase = dustSeed * 6.28318530718;
      vec3 transformed = position;
      float drift = dustTime * (0.085 + dustSeed * 0.065);
      float closeOrbit = dustTime * (0.18 + dustSeed * 0.11);
      transformed.x += sin(drift + phase) * (1.12 + dustSeed * 1.08);
      transformed.y += sin(dustTime * (0.18 + dustSeed * 0.08) + phase * 1.7) * 0.52;
      transformed.z += cos(drift * 0.92 + phase * 1.3) * (1.08 + dustSeed * 0.96);
      transformed.x += cos(closeOrbit + phase * 2.1) * 0.74;
      transformed.z += sin(closeOrbit * 0.86 + phase * 2.4) * 0.74;
      vec4 mvPosition = modelViewMatrix * vec4(transformed, 1.0);
      float distanceToCamera = max(1.0, -mvPosition.z);
      float close = 1.0 - smoothstep(9.0, 34.0, distanceToCamera);
      float far = smoothstep(120.0, 18.0, distanceToCamera);
      vDustBlur = mix(0.22, 0.74, close);
      float shimmer = 0.82 + 0.18 * sin(dustTime * (0.42 + dustSeed * 0.28) + phase * 2.8);
      vDustAlpha = (0.05 + dustSeed * 0.12) * far * mix(0.42, 0.82, close) * shimmer;
      gl_PointSize = clamp(dustSize * (150.0 / distanceToCamera) * mix(1.0, 6.4, close), 0.8, 54.0);
      gl_Position = projectionMatrix * mvPosition;
    }
  `,
  fragmentShader: `
    varying float vDustAlpha;
    varying float vDustBlur;
    void main() {
      vec2 point = gl_PointCoord - vec2(0.5);
      float radius = length(point) * 2.0;
      float disc = 1.0 - smoothstep(vDustBlur, 1.0, radius);
      float core = 1.0 - smoothstep(0.0, max(0.18, vDustBlur), radius);
      float alpha = (disc * 0.72 + core * 0.16) * vDustAlpha;
      gl_FragColor = vec4(0.67, 1.0, 0.9, alpha);
    }
  `,
};

const warningDeformSnippet = `
float warningPhase = warningSeedValue * 37.17;
vec3 warningSeedVector = normalize(vec3(
  fract(warningSeedValue * 12.9898) - 0.5,
  fract(warningSeedValue * 78.233) - 0.5,
  fract(warningSeedValue * 45.164) - 0.5
) + vec3(0.12, 0.28, 0.44));
vec3 warningDirection = normalize(position + normal * (0.34 + warningSeedValue * 0.28) + warningSeedVector * 0.18);
float warningWave = sin(warningTime * (4.3 + warningSeedValue * 1.8) + warningPhase + dot(warningDirection, vec3(13.0, 7.0, 17.0)));
float warningShard = sin(warningTime * (3.0 + warningSeedValue * 1.4) + warningPhase + warningDirection.x * 23.0)
  * sin(warningTime * (3.9 + warningSeedValue * 1.1) + warningDirection.y * 19.0)
  * sin(warningTime * (4.5 + warningSeedValue * 1.6) + warningDirection.z * 29.0);
float warningSpike = smoothstep(0.58, 0.98, warningWave * 0.5 + 0.5) * 0.34
  + smoothstep(0.42, 0.92, warningShard * 0.5 + 0.5) * 0.26;
transformed += warningDirection * warningSpike;
transformed += normal * sin(warningTime * (7.0 + warningSeedValue * 4.0) + warningPhase + length(position.xyz) * 31.0) * 0.04;`;

class ClusterMapController {
  private viewport: HTMLElement;
  private tooltip: HTMLElement | null;
  private loading: HTMLElement | null;
  private inspector: HTMLElement | null;
  private renderer: THREE.WebGLRenderer;
  private composer: EffectComposer;
  private bloomPass: UnrealBloomPass;
  private bokehPass: BokehPass;
  private vignettePass: ShaderPass;
  private scene = new THREE.Scene();
  private camera = new THREE.PerspectiveCamera(48, 1, 0.1, 1200);
  private controls: OrbitControls;
  private lastRenderInfo: THREE.WebGLInfo["render"] = { frame: 0, calls: 0, triangles: 0, points: 0, lines: 0 };
  private labelLayer = document.createElement("div");
  private raycaster = new THREE.Raycaster();
  private pointer = new THREE.Vector2();
  private focusPointer = new THREE.Vector2();
  private focusPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
  private focusPoint = new THREE.Vector3();
  private hasPointerFocus = false;
  private pointerDown: { x: number; y: number } | null = null;
  private pointerMovedSinceDown = false;
  private frame = 0;
  private refreshTimer = 0;
  private resizeObserver: ResizeObserver;
  private abortController: AbortController | null = null;
  private paused = false;
  private autorotate = false;
  private selectedItemId: string | null = null;
  private hovered: THREE.Object3D | null = null;
  private hoveredItemId: string | null = null;
  private interactive: THREE.Object3D[] = [];
  private objectsById = new Map<string, THREE.Object3D>();
  private labelsById = new Map<string, HTMLElement>();
  private connectors: THREE.Group[] = [];
  private instancedCollections = new Map<InstancedPrimitiveType, InstancedCollection>();
  private particleGeometry = new THREE.BufferGeometry();
  private particleMaterial = new THREE.ShaderMaterial({
    uniforms: THREE.UniformsUtils.clone(trafficSparkShader.uniforms),
    vertexShader: trafficSparkShader.vertexShader,
    fragmentShader: trafficSparkShader.fragmentShader,
    transparent: true,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  private particlePoints = new THREE.Points(this.particleGeometry, this.particleMaterial);
  private trafficBeamMaterial = new THREE.ShaderMaterial({
    uniforms: THREE.UniformsUtils.clone(trafficBeamShader.uniforms),
    vertexShader: trafficBeamShader.vertexShader,
    fragmentShader: trafficBeamShader.fragmentShader,
    transparent: true,
    side: THREE.DoubleSide,
    depthTest: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  private dustMaterial = new THREE.ShaderMaterial({
    uniforms: THREE.UniformsUtils.clone(dustShader.uniforms),
    vertexShader: dustShader.vertexShader,
    fragmentShader: dustShader.fragmentShader,
    transparent: true,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  private dustPoints = new THREE.Points(createDustGeometry(), this.dustMaterial);
  private pulses: THREE.Object3D[] = [];
  private selectionEffect: THREE.Group | null = null;
  private focusTween: FocusTween | null = null;
  private data: ClusterMapData | null = null;
  private currentLayout: MapLayoutItem[] = [];
  private endpoint = "";
  private hasFramedScene = false;
  private pendingHistorySelectionId: string | null = null;
  private homeCameraPosition = new THREE.Vector3(24, 22, 28);
  private homeTarget = new THREE.Vector3(0, 0, 0);

  constructor(private shell: HTMLElement) {
    const viewport = shell.querySelector<HTMLElement>("[data-map-viewport]");
    if (!viewport) {
      throw new Error("Map viewport is missing.");
    }
    this.viewport = viewport;
    this.tooltip = shell.querySelector<HTMLElement>("[data-map-tooltip]");
    this.loading = shell.querySelector<HTMLElement>("[data-map-loading]");
    this.inspector = shell.querySelector<HTMLElement>("[data-map-inspector]");
    this.selectedItemId = readMapSelectedFromURL();
    this.pendingHistorySelectionId = this.selectedItemId;
    this.shell.dataset.mapSelected = this.selectedItemId || "";
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.info.autoReset = false;
    this.renderer.domElement.className = "map-canvas";
    this.composer = new EffectComposer(this.renderer);
    this.composer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.75));
    this.composer.addPass(new RenderPass(this.scene, this.camera));
    this.bloomPass = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.32, 0.18, 0.78);
    this.bloomPass.enabled = true;
    this.composer.addPass(this.bloomPass);
    this.bokehPass = new BokehPass(this.scene, this.camera, {
      focus: 34,
      aperture: 0.000095,
      maxblur: 0.036,
    });
    this.wrapBokehDepthPass();
    this.bokehPass.enabled = true;
    this.composer.addPass(this.bokehPass);
    this.vignettePass = new ShaderPass(vignetteShader);
    this.vignettePass.enabled = true;
    this.composer.addPass(this.vignettePass);
    this.composer.addPass(new OutputPass());
    this.labelLayer.className = "map-label-layer";
    this.viewport.append(this.renderer.domElement);
    this.viewport.append(this.labelLayer);

    this.camera.position.set(24, 22, 28);
    this.controls = new OrbitControls(this.camera, this.renderer.domElement);
    this.controls.enableDamping = true;
    this.controls.dampingFactor = 0.08;
    this.controls.autoRotateSpeed = 0.35;
    this.controls.maxPolarAngle = Math.PI * 0.47;
    this.controls.minDistance = 10;
    this.controls.maxDistance = 90;

    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(this.viewport);
    this.renderer.domElement.addEventListener("pointerdown", this.onPointerDown);
    this.renderer.domElement.addEventListener("pointermove", this.onPointerMove);
    this.renderer.domElement.addEventListener("pointerup", this.onPointerUp);
    this.renderer.domElement.addEventListener("pointercancel", this.onPointerCancel);
    this.renderer.domElement.addEventListener("pointerleave", this.onPointerLeave);
    this.renderer.domElement.addEventListener("click", this.onClick);
    shell.addEventListener("click", this.onShellClick);
    window.addEventListener("kubernetto:map-selection-popstate", this.onSelectionPopstate);

    this.buildBaseScene();
    this.resize();
    void this.load();
    this.refreshTimer = window.setInterval(() => void this.load(), 5000);
    this.animate();
  }

  syncShell() {
    const previousEndpoint = this.endpoint;
    this.refreshRefs();
    if (!this.renderer.domElement.isConnected && this.viewport) {
      this.viewport.append(this.renderer.domElement);
    }
    if (!this.labelLayer.isConnected && this.viewport) {
      this.viewport.append(this.labelLayer);
    }
    this.syncControlButtons();
    const nextEndpoint = this.shell.dataset.endpoint || "/ui/map-data";
    if (nextEndpoint !== previousEndpoint) {
      this.endpoint = nextEndpoint;
      this.hasFramedScene = false;
      this.selectedItemId = null;
      this.pendingHistorySelectionId = null;
      this.shell.dataset.mapSelected = "";
      this.clearTopology();
      this.renderSidebar();
      void this.load();
      return;
    }
    this.resize();
  }

  debugStats() {
    const bokehUniforms = this.bokehPass.uniforms as { focus?: { value: number }; aperture?: { value: number }; maxblur?: { value: number } };
    return {
      render: { ...this.lastRenderInfo },
      controls: {
        autorotate: this.autorotate,
        autorotateActive: this.controls.autoRotate,
        paused: this.paused,
        camera: this.camera.position.toArray().map((value) => Number(value.toFixed(3))),
        target: this.controls.target.toArray().map((value) => Number(value.toFixed(3))),
      },
      focus: {
        value: Number((bokehUniforms.focus?.value || 0).toFixed(3)),
        target: Number(this.currentFocusDistance().toFixed(3)),
        mouse: this.hasPointerFocus,
        hovered: this.hoveredItemId,
        selected: this.selectedItemId,
        aperture: Number((bokehUniforms.aperture?.value || 0).toFixed(6)),
        maxblur: Number((bokehUniforms.maxblur?.value || 0).toFixed(3)),
      },
      sceneDrawables: sceneDrawableStats(this.scene),
      connectorBatches: this.connectors.length,
      instancedCollections: [...this.instancedCollections.entries()].map(([type, collection]) => ({
        type,
        count: collection.mesh.count,
      })),
      sceneChildren: this.scene.children.length,
    };
  }

  destroy() {
    window.clearInterval(this.refreshTimer);
    window.cancelAnimationFrame(this.frame);
    this.abortController?.abort();
    this.resizeObserver.disconnect();
    this.renderer.domElement.removeEventListener("pointerdown", this.onPointerDown);
    this.renderer.domElement.removeEventListener("pointermove", this.onPointerMove);
    this.renderer.domElement.removeEventListener("pointerup", this.onPointerUp);
    this.renderer.domElement.removeEventListener("pointercancel", this.onPointerCancel);
    this.renderer.domElement.removeEventListener("pointerleave", this.onPointerLeave);
    this.renderer.domElement.removeEventListener("click", this.onClick);
    this.shell.removeEventListener("click", this.onShellClick);
    window.removeEventListener("kubernetto:map-selection-popstate", this.onSelectionPopstate);
    this.controls.dispose();
    this.disposeScene();
    this.trafficBeamMaterial.dispose();
    this.composer.dispose();
    this.renderer.dispose();
    this.renderer.domElement.remove();
  }

  private async load() {
    const endpoint = this.shell.dataset.endpoint || "/ui/map-data";
    const initialLoad = !this.data;
    this.endpoint = endpoint;
    this.abortController?.abort();
    this.abortController = new AbortController();
    this.setLoading(initialLoad);
    try {
      const response = await fetch(endpoint, {
        headers: { Accept: "application/json" },
        signal: this.abortController.signal,
      });
      const body = await response.json();
      if (!response.ok) {
        throw new Error(body?.error || "Map data is unavailable.");
      }
      this.data = body as ClusterMapData;
      this.renderData();
      this.updateStats();
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        this.renderError(error instanceof Error ? error.message : "Map data is unavailable.");
      }
    } finally {
      this.setLoading(false);
    }
  }

  private refreshRefs() {
    const nextViewport = this.shell.querySelector<HTMLElement>("[data-map-viewport]");
    if (nextViewport && nextViewport !== this.viewport) {
      this.resizeObserver.unobserve(this.viewport);
      this.viewport = nextViewport;
      this.viewport.append(this.renderer.domElement);
      this.resizeObserver.observe(this.viewport);
    }
    this.tooltip = this.shell.querySelector<HTMLElement>("[data-map-tooltip]");
    this.loading = this.shell.querySelector<HTMLElement>("[data-map-loading]");
    this.inspector = this.shell.querySelector<HTMLElement>("[data-map-inspector]");
    this.syncControlButtons();
  }

  private wrapBokehDepthPass() {
    const renderBokeh = this.bokehPass.render.bind(this.bokehPass);
    this.bokehPass.render = ((renderer, writeBuffer, readBuffer, deltaTime, maskActive) => {
      const hidden: Array<{ object: THREE.Object3D; visible: boolean }> = [];
      this.scene.traverse((object) => {
        if (!object.userData.excludeFromBokehDepth) {
          return;
        }
        hidden.push({ object, visible: object.visible });
        object.visible = false;
      });
      try {
        renderBokeh(renderer, writeBuffer, readBuffer, deltaTime, maskActive);
      } finally {
        for (const entry of hidden) {
          entry.object.visible = entry.visible;
        }
      }
    }) as BokehPass["render"];
  }

  private syncControlButtons() {
    this.syncPauseButton(this.shell.querySelector<HTMLElement>("[data-map-toggle]"));
    this.syncAutorotateButton(this.shell.querySelector<HTMLElement>("[data-map-autorotate]"));
  }

  private syncPauseButton(button: HTMLElement | null) {
    button?.setAttribute("aria-pressed", String(this.paused));
    button?.setAttribute("aria-label", this.paused ? "Resume animation" : "Pause animation");
    button?.setAttribute("title", this.paused ? "Resume animation" : "Pause animation");
    const icon = button?.querySelector("span");
    if (icon) {
      icon.className = this.paused ? "icon-[lucide--play]" : "icon-[lucide--pause]";
    }
  }

  private syncAutorotateButton(button: HTMLElement | null) {
    const motionReduced = Boolean(reducedMotion?.matches);
    if (motionReduced) {
      this.autorotate = false;
    }
    button?.setAttribute("aria-pressed", String(this.autorotate));
    button?.setAttribute(
      "aria-label",
      motionReduced ? "Slow autorotation disabled by reduced motion" : this.autorotate ? "Stop slow autorotation" : "Start slow autorotation",
    );
    button?.setAttribute(
      "title",
      motionReduced ? "Slow autorotation disabled by reduced motion" : this.autorotate ? "Stop slow autorotation" : "Start slow autorotation",
    );
    button?.toggleAttribute("disabled", motionReduced);
    const icon = button?.querySelector("span");
    if (icon) {
      icon.className = this.autorotate ? "icon-[lucide--circle-pause]" : "icon-[lucide--rotate-cw]";
    }
  }

  private buildBaseScene() {
    this.scene.background = new THREE.Color(0x071110);
    this.scene.fog = new THREE.Fog(0x0a1e1b, 95, 300);
    this.scene.add(new THREE.AmbientLight(0x7ee7d3, 0.9));
    const keyLight = new THREE.DirectionalLight(0xa7fff0, 2.1);
    keyLight.position.set(14, 28, 16);
    this.scene.add(keyLight);
    const rimLight = new THREE.PointLight(0x67e8f9, 2.2, 80);
    rimLight.position.set(-20, 12, -18);
    this.scene.add(rimLight);

    const grid = new THREE.GridHelper(90, 45, 0x2dd4bf, 0x164e44);
    grid.position.y = -0.03;
    this.scene.add(grid);
    this.trafficBeamMaterial.userData.sharedSceneMaterial = true;
    this.particlePoints.userData.particles = true;
    this.particlePoints.frustumCulled = false;
    this.dustPoints.userData.dust = true;
    this.dustPoints.frustumCulled = false;
    this.scene.add(this.dustPoints);
  }

  private renderData() {
    if (!this.data) {
      return;
    }
    this.clearTopology();
    const layout = buildMapLayout(this.data);
    this.currentLayout = layout;
    const connectedPairs = new Set<string>();
    const trafficBeamGeometries: THREE.BufferGeometry[] = [];
    const trafficBranchGeometries: THREE.BufferGeometry[] = [];
    this.updateHomeView(layout);
    this.prepareInstancedCollections(layout);

    for (const item of layout) {
      const object = this.objectForItem(item);
      object.position.set(item.x, item.y, item.z);
      object.userData.item = item;
      object.userData.baseY = item.y;
      this.objectsById.set(item.id, object);
      const label = labelForItem(item);
      this.labelsById.set(item.id, label);
      this.labelLayer.append(label);
      this.interactive.push(object);
      this.scene.add(object);
      if (item.type === "warning") {
        this.pulses.push(object);
      }
    }

    for (const item of layout) {
      for (const targetID of item.trafficTargetIds) {
        this.addConnector(item.id, targetID, "traffic", connectedPairs, trafficBeamGeometries, trafficBranchGeometries);
      }
    }
    this.addBatchedConnectors(trafficBeamGeometries, trafficBranchGeometries);
    this.rebuildParticleFlow();

    if (this.data.truncated) {
      this.shell.classList.add("map-truncated");
    } else {
      this.shell.classList.remove("map-truncated");
    }
    if (!this.hasFramedScene) {
      this.resetCamera(false);
      this.hasFramedScene = true;
    }
    this.restoreSelectedItem();
  }

  private addConnector(
    fromID: string,
    toID: string,
    relation: MapConnectorRelation,
    connectedPairs: Set<string>,
    trafficBeamGeometries: THREE.BufferGeometry[],
    trafficBranchGeometries: THREE.BufferGeometry[],
  ) {
    const from = this.objectsById.get(fromID);
    const to = this.objectsById.get(toID);
    if (!from || !to) {
      return;
    }
    const pairKey = [fromID, toID].sort().join("->");
    if (connectedPairs.has(pairKey)) {
      return;
    }
    connectedPairs.add(pairKey);
    const geometries = connectorGeometries(from.position, to.position, pairKey);
    trafficBeamGeometries.push(geometries.beam);
    trafficBranchGeometries.push(geometries.branches);
    void relation;
  }

  private addBatchedConnectors(trafficBeamGeometries: THREE.BufferGeometry[], trafficBranchGeometries: THREE.BufferGeometry[]) {
    if (trafficBeamGeometries.length === 0 && trafficBranchGeometries.length === 0) {
      return;
    }
    const group = new THREE.Group();
    group.userData.connector = true;
    group.userData.excludeFromBokehDepth = true;
    group.renderOrder = 90;
    const beamGeometry = trafficBeamGeometries.length > 0 ? mergeGeometries(trafficBeamGeometries, false) : null;
    const branchGeometry = trafficBranchGeometries.length > 0 ? mergeGeometries(trafficBranchGeometries, false) : null;
    if (beamGeometry) {
      const beam = new THREE.Mesh(beamGeometry, this.trafficBeamMaterial);
      beam.frustumCulled = false;
      beam.renderOrder = 90;
      group.add(beam);
    }
    if (branchGeometry) {
      const branches = new THREE.LineSegments(branchGeometry, this.trafficBeamMaterial);
      branches.frustumCulled = false;
      branches.renderOrder = 91;
      group.add(branches);
    }
    trafficBeamGeometries.forEach((geometry) => geometry.dispose());
    trafficBranchGeometries.forEach((geometry) => geometry.dispose());
    this.connectors.push(group);
    this.scene.add(group);
  }

  private prepareInstancedCollections(layout: MapLayoutItem[]) {
    const counts = new Map<InstancedPrimitiveType, number>();
    for (const item of layout) {
      const type = instancedPrimitiveType(item);
      if (type) {
        counts.set(type, (counts.get(type) || 0) + 1);
      }
    }
    for (const [type, count] of counts) {
      const geometry = instancedGeometryFor(type);
      const material = instancedMaterialFor(type);
      const warningSeeds = type === "warning" ? new THREE.InstancedBufferAttribute(new Float32Array(count), 1) : undefined;
      if (warningSeeds) {
        geometry.setAttribute("warningSeed", warningSeeds);
      }
      const mesh = new THREE.InstancedMesh(geometry, material, count);
      mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
      mesh.frustumCulled = false;
      mesh.userData.instancedCollection = true;
      this.instancedCollections.set(type, { mesh, geometry, nextIndex: 0, warningSeeds });
      this.scene.add(mesh);
    }
  }

  private objectForItem(item: MapLayoutItem): THREE.Object3D {
    const instancedObject = this.instancedObjectForItem(item);
    if (instancedObject) {
      return instancedObject;
    }
    const color = colorFor(item.statusKey, item.type);
    const material = solidMaterial(color, item.type === "warning" ? 0.94 : 0.82);
    const wireMaterial = wireMaterialFor(color);
    switch (item.type) {
      case "namespace": {
        const group = new THREE.Group();
        const platform = new THREE.Mesh(new THREE.CylinderGeometry(item.size, item.size * 0.92, 0.16, 8), solidMaterial(0x0f2c28, 0.5));
        platform.position.y = -0.05;
        const edge = new THREE.LineSegments(new THREE.EdgesGeometry(platform.geometry), wireMaterial);
        const halo = new THREE.Mesh(new THREE.TorusGeometry(item.size * 0.92, 0.035, 6, 64), solidMaterial(color, 0.58));
        halo.rotation.x = Math.PI / 2;
        halo.position.y = 0.08;
        group.add(platform, edge, halo);
        return group;
      }
      case "category": {
        const group = new THREE.Group();
        const slab = new THREE.Mesh(new THREE.CylinderGeometry(item.size * 0.84, item.size, 0.32, 6), solidMaterial(0x102b32, 0.62));
        const edge = new THREE.LineSegments(new THREE.EdgesGeometry(slab.geometry), wireMaterial);
        const crown = new THREE.Mesh(new THREE.TorusGeometry(item.size * 0.72, 0.045, 6, 48), solidMaterial(color, 0.7));
        crown.rotation.x = Math.PI / 2;
        crown.position.y = 0.24;
        group.add(slab, edge, crown);
        return group;
      }
      case "clusterResource":
        return new THREE.Mesh(new THREE.DodecahedronGeometry(item.size, 0), material);
      case "node":
        return new THREE.Mesh(new THREE.BoxGeometry(1.55, Math.max(2.6, item.size), 1.55), material);
      case "workload": {
        const mesh = new THREE.Mesh(new THREE.BoxGeometry(item.size, item.size * 0.78, item.size), solidMaterial(0x0b1b18, 0.34));
        const edge = new THREE.LineSegments(new THREE.EdgesGeometry(mesh.geometry), wireMaterial);
        const group = new THREE.Group();
        group.add(mesh, edge);
        return group;
      }
      case "service":
        return new THREE.Mesh(new THREE.OctahedronGeometry(item.size, 1), material);
      case "warning":
        return new THREE.Mesh(new THREE.SphereGeometry(item.size, 20, 16), material);
      case "pod":
      default:
        return new THREE.Mesh(new THREE.SphereGeometry(item.size, 18, 14), material);
    }
  }

  private instancedObjectForItem(item: MapLayoutItem): THREE.Object3D | null {
    const type = instancedPrimitiveType(item);
    const collection = type ? this.instancedCollections.get(type) : undefined;
    if (!type || !collection) {
      return null;
    }
    const index = collection.nextIndex;
    collection.nextIndex += 1;
    const position = new THREE.Vector3(item.x, item.y, item.z);
    const baseScale = instancedBaseScaleFor(item);
    const baseColor = new THREE.Color(colorFor(item.statusKey, item.type));
    const instance: InstancedInstance = {
      mesh: collection.mesh,
      index,
      position,
      baseScale,
      baseColor,
    };
    if (collection.warningSeeds) {
      collection.warningSeeds.setX(index, seededUnitValue(item.id));
      collection.warningSeeds.needsUpdate = true;
    }
    writeInstancedInstance(instance, baseScale, baseColor);
    const proxy = new THREE.Mesh(collection.geometry, instancedProxyMaterial);
    proxy.visible = false;
    proxy.position.copy(position);
    proxy.scale.copy(baseScale);
    proxy.userData.instancedInstance = instance;
    proxy.userData.skipDispose = true;
    proxy.updateMatrixWorld(true);
    return proxy;
  }

  private updateStats() {
    if (!this.data) {
      return;
    }
    for (const [key, value] of Object.entries(this.data.counts)) {
      const target = this.shell.querySelector<HTMLElement>(`[data-map-count="${key}"]`);
      if (target) {
        target.textContent = String(value);
      }
    }
  }

  private renderError(message: string) {
    if (!this.inspector) {
      return;
    }
    this.inspector.replaceChildren();
    const block = document.createElement("div");
    const label = document.createElement("div");
    label.className = "map-inspector-label";
    label.textContent = "Map unavailable";
    const strong = document.createElement("strong");
    strong.textContent = message;
    block.append(label, strong);
    this.inspector.append(block);
  }

  private renderSidebar(selectedItem?: MapLayoutItem) {
    if (!this.inspector) {
      return;
    }
    this.inspector.replaceChildren();
    const summary = document.createElement("div");
    summary.className = "map-inspector-summary";
    const label = document.createElement("div");
    label.className = "map-inspector-label";
    label.textContent = selectedItem ? selectedItem.type : "Selected";
    const strong = document.createElement("strong");
    strong.textContent = selectedItem?.label || "Cluster topology";
    const span = document.createElement("span");
    span.textContent = selectedItem?.detail || "Choose a resource from the topology tree.";
    summary.append(label, strong, span);
    if (selectedItem?.href) {
      const link = document.createElement("a");
      link.href = selectedItem.href;
      link.textContent = "Open details";
      summary.append(link);
    }
    this.inspector.append(summary, this.renderOperationalInsights(selectedItem), this.renderTopologyTree(selectedItem));
  }

  private renderOperationalInsights(selectedItem?: MapLayoutItem) {
    const section = document.createElement("section");
    section.className = "map-insights";
    section.setAttribute("aria-label", selectedItem ? `Operational context for ${selectedItem.label}` : "Cluster attention");
    const header = document.createElement("div");
    header.className = "map-insights-header";
    const title = document.createElement("strong");
    title.textContent = selectedItem ? "Blast radius" : "Needs attention";
    const subtitle = document.createElement("span");
    const groups = selectedItem ? this.selectedInsightGroups(selectedItem) : this.clusterInsightGroups();
    const rowCount = groups.reduce((count, group) => count + group.items.length, 0);
    subtitle.textContent = selectedItem ? `${rowCount} related` : `${rowCount} signals`;
    header.append(title, subtitle);
    section.append(header);

    if (groups.length === 0) {
      const empty = document.createElement("div");
      empty.className = "map-insights-empty";
      empty.textContent = selectedItem ? "No immediate operational relationships found." : "No warnings or unhealthy resources in the visible map.";
      section.append(empty);
      return section;
    }

    for (const group of groups) {
      section.append(this.renderInsightGroup(group.title, group.items));
    }
    return section;
  }

  private renderInsightGroup(title: string, items: MapLayoutItem[]) {
    const group = document.createElement("div");
    group.className = "map-insight-group";
    const header = document.createElement("div");
    header.className = "map-insight-group-header";
    const strong = document.createElement("strong");
    strong.textContent = title;
    const count = document.createElement("span");
    count.textContent = String(items.length);
    header.append(strong, count);
    group.append(header);
    for (const item of items.slice(0, 8)) {
      group.append(this.renderInsightRow(item));
    }
    if (items.length > 8) {
      const more = document.createElement("div");
      more.className = "map-insight-more";
      more.textContent = `${items.length - 8} more`;
      group.append(more);
    }
    return group;
  }

  private renderInsightRow(item: MapLayoutItem) {
    const row = document.createElement("button");
    row.type = "button";
    row.className = `map-insight-item is-${item.statusKey}`;
    row.dataset.mapTreeId = item.id;
    row.title = `${item.label} · ${item.detail}`;
    const icon = document.createElement("span");
    icon.className = `map-insight-icon ${treeKindIconClass(item)}`;
    icon.setAttribute("aria-hidden", "true");
    const body = document.createElement("span");
    body.className = "map-insight-body";
    const name = document.createElement("span");
    name.className = "map-insight-name";
    name.textContent = item.label;
    const detail = document.createElement("span");
    detail.className = "map-insight-detail";
    detail.textContent = item.detail;
    body.append(name, detail);
    const status = document.createElement("span");
    status.className = "map-insight-status";
    status.textContent = statusLabel(item.statusKey);
    row.append(icon, body, status);
    return row;
  }

  private selectedInsightGroups(item: MapLayoutItem) {
    const byId = new Map(this.currentLayout.map((candidate) => [candidate.id, candidate]));
    const ownedScope = this.relationshipScopeIds(item, byId);
    const trafficOutIds = new Set<string>();
    const trafficInIds = new Set<string>();
    for (const scopedId of ownedScope) {
      const scoped = byId.get(scopedId);
      scoped?.trafficTargetIds.forEach((id) => trafficOutIds.add(id));
    }
    for (const candidate of this.currentLayout) {
      if (ownedScope.has(candidate.id)) {
        continue;
      }
      if (candidate.trafficTargetIds.some((id) => ownedScope.has(id))) {
        trafficInIds.add(candidate.id);
      }
    }
    const ownerIds = this.currentLayout
      .filter((candidate) => candidate.ownedIds.includes(item.id) || ((item.type === "pod" || item.type === "warning") && item.targetIds.includes(candidate.id)))
      .map((candidate) => candidate.id);
    const warningIds = this.currentLayout
      .filter((candidate) => candidate.type === "warning" && candidate.targetIds.some((id) => ownedScope.has(id) || trafficOutIds.has(id)))
      .map((candidate) => candidate.id);
    const groups = [
      { title: "Warnings nearby", items: this.itemsForIds(warningIds, byId) },
      { title: "Owned by", items: this.itemsForIds(ownerIds, byId) },
      { title: "Owns", items: this.itemsForIds(item.ownedIds, byId) },
      { title: "Traffic in", items: this.itemsForIds(trafficInIds, byId) },
      { title: "Traffic out", items: this.itemsForIds(trafficOutIds, byId) },
    ];
    return groups.filter((group) => group.items.length > 0);
  }

  private clusterInsightGroups() {
    const attention = this.currentLayout
      .filter((item) => item.statusKey === "danger" || item.statusKey === "warn" || serviceHasNoTargets(item))
      .sort(compareInsightItems);
    const warnings = attention.filter((item) => item.type === "warning");
    const unhealthy = attention.filter((item) => item.type !== "warning" && item.statusKey !== "neutral");
    const emptyTraffic = attention.filter((item) => serviceHasNoTargets(item));
    return [
      { title: "Warnings", items: warnings },
      { title: "Unhealthy", items: unhealthy },
      { title: "No traffic targets", items: emptyTraffic },
    ].filter((group) => group.items.length > 0);
  }

  private relationshipScopeIds(item: MapLayoutItem, byId: Map<string, MapLayoutItem>) {
    const scope = new Set<string>([item.id]);
    const visit = (id: string) => {
      const current = byId.get(id);
      if (!current) {
        return;
      }
      for (const childId of current.ownedIds) {
        if (scope.has(childId)) {
          continue;
        }
        scope.add(childId);
        visit(childId);
      }
    };
    visit(item.id);
    return scope;
  }

  private itemsForIds(ids: Iterable<string>, byId: Map<string, MapLayoutItem>) {
    const seen = new Set<string>();
    const items: MapLayoutItem[] = [];
    for (const id of ids) {
      if (seen.has(id)) {
        continue;
      }
      const item = byId.get(id);
      if (!item) {
        continue;
      }
      seen.add(id);
      items.push(item);
    }
    return items.sort(compareInsightItems);
  }

  private renderTopologyTree(selectedItem?: MapLayoutItem) {
    const section = document.createElement("section");
    section.className = "map-tree";
    section.setAttribute("aria-label", selectedItem ? `Resources inside ${selectedItem.label}` : "Top-level topology resources");
    const header = document.createElement("div");
    header.className = "map-tree-header";
    const title = document.createElement("strong");
    title.textContent = selectedItem ? "Contains" : "Top level";
    const count = document.createElement("span");
    const rows = buildTopologyTree(this.currentLayout, defaultTopologyTreeLimit, selectedItem?.id || "");
    count.textContent = `${rows.length} ${rows.length === 1 ? "resource" : "resources"}`;
    header.append(title, count);

    const list = document.createElement("div");
    list.className = "map-tree-list";
    list.setAttribute("role", "table");
    const tableHeader = document.createElement("div");
    tableHeader.className = "map-tree-table-header";
    tableHeader.setAttribute("role", "row");
    const kindHeader = document.createElement("span");
    kindHeader.className = "map-tree-kind-header";
    kindHeader.setAttribute("role", "columnheader");
    kindHeader.setAttribute("aria-label", "Kind");
    kindHeader.title = "Kind";
    const kindHeaderIcon = document.createElement("span");
    kindHeaderIcon.className = "icon-[uil--cube]";
    kindHeaderIcon.setAttribute("aria-hidden", "true");
    kindHeader.append(kindHeaderIcon);
    tableHeader.append(kindHeader);
    for (const text of ["Resource", "Status"]) {
      const cell = document.createElement("span");
      cell.setAttribute("role", "columnheader");
      cell.textContent = text;
      tableHeader.append(cell);
    }
    list.append(tableHeader);
    for (const row of rows) {
      list.append(this.renderTopologyTreeRow(row.item, row.depth));
    }
    if (rows.length === 0) {
      const empty = document.createElement("div");
      empty.className = "map-tree-empty";
      empty.textContent = selectedItem ? "No resources inside this object." : "No top-level resources.";
      list.append(empty);
    }
    const hiddenCount = selectedItem ? 0 : Math.max(0, this.currentLayout.length - rows.length);
    if (hiddenCount > 0 && rows.length >= defaultTopologyTreeLimit) {
      const more = document.createElement("div");
      more.className = "map-tree-more";
      more.textContent = `${hiddenCount} more resources hidden`;
      list.append(more);
    }
    section.append(header, list);
    return section;
  }

  private renderTopologyTreeRow(item: MapLayoutItem, depth: number) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `map-tree-item is-${item.statusKey}`;
    button.dataset.mapTreeId = item.id;
    button.style.setProperty("--depth", String(Math.min(depth, 6)));
    button.setAttribute("aria-current", String(item.id === this.selectedItemId));
    button.setAttribute("role", "row");
    button.title = `${item.label} · ${item.detail}`;

    const kind = document.createElement("span");
    kind.className = "map-tree-kind";
    kind.setAttribute("role", "cell");
    kind.setAttribute("aria-label", treeKindLabel(item));
    kind.title = treeKindLabel(item);
    const kindIcon = document.createElement("span");
    kindIcon.className = treeKindIconClass(item);
    kindIcon.setAttribute("aria-hidden", "true");
    kind.append(kindIcon);
    const body = document.createElement("span");
    body.className = "map-tree-body";
    body.setAttribute("role", "cell");
    const name = document.createElement("span");
    name.className = "map-tree-name";
    name.textContent = item.label;
    const detail = document.createElement("span");
    detail.className = "map-tree-detail";
    detail.textContent = item.detail;
    body.append(name, detail);
    const status = document.createElement("span");
    status.className = "map-tree-status";
    status.setAttribute("role", "cell");
    status.textContent = statusLabel(item.statusKey);
    button.append(kind, body, status);
    return button;
  }

  private setLoading(loading: boolean) {
    if (this.loading) {
      this.loading.hidden = !loading;
    }
  }

  private resize() {
    const rect = this.viewport.getBoundingClientRect();
    const width = Math.max(1, Math.floor(rect.width));
    const height = Math.max(1, Math.floor(rect.height));
    this.renderer.setSize(width, height, false);
    this.composer.setSize(width, height);
    this.bloomPass.setSize(width, height);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
  }

  private animate = () => {
    this.frame = window.requestAnimationFrame(this.animate);
    if (!this.paused) {
      const elapsed = performance.now() * 0.001;
      for (const object of this.pulses) {
        object.position.y = (object.userData.baseY || 2.35) + Math.sin(elapsed * 3.4) * 0.18;
        const pulseScale = 1 + Math.sin(elapsed * 4.2) * 0.12;
        const instance = object.userData.instancedInstance as InstancedInstance | undefined;
        if (instance) {
          object.scale.copy(instance.baseScale).multiplyScalar(pulseScale);
          writeInstancedInstance(instance, object.scale, instance.baseColor, object.position);
        } else {
          object.scale.setScalar(pulseScale);
        }
      }
      this.updateWarningShaderTime(elapsed);
      this.updateSelectionShaderTime(elapsed);
      this.updateTrafficBeamTime(elapsed);
      this.updateParticleFlowTime(elapsed);
      this.updateDust(elapsed);
    }
    this.updateFocusTween();
    this.controls.autoRotate = this.autorotate && !this.paused && !this.focusTween && !reducedMotion?.matches;
    this.controls.update();
    this.updatePostprocessingFocus();
    this.updateLabels();
    this.renderer.info.reset();
    this.composer.render();
    this.lastRenderInfo = { ...this.renderer.info.render };
  };

  private updateWarningShaderTime(elapsed: number) {
    const material = this.instancedCollections.get("warning")?.mesh.material as THREE.Material | undefined;
    const shader = material?.userData.warningShader as { uniforms?: { warningTime?: { value: number } } } | undefined;
    const uniform = shader?.uniforms?.warningTime;
    if (uniform) {
      uniform.value = elapsed;
    }
  }

  private updateSelectionShaderTime(elapsed: number) {
    if (!this.selectionEffect) {
      return;
    }
    this.selectionEffect.traverse((object) => {
      const material = (object as THREE.Mesh).material as THREE.Material | THREE.Material[] | undefined;
      const materials = Array.isArray(material) ? material : material ? [material] : [];
      for (const item of materials) {
        const warningShader = item.userData.warningShader as { uniforms?: { warningTime?: { value: number } } } | undefined;
        const spotlightShader = item.userData.spotlightShader as { uniforms?: { spotlightTime?: { value: number } } } | undefined;
        if (warningShader?.uniforms?.warningTime) {
          warningShader.uniforms.warningTime.value = elapsed;
        }
        if (spotlightShader?.uniforms?.spotlightTime) {
          spotlightShader.uniforms.spotlightTime.value = elapsed;
        }
      }
    });
  }

  private updateTrafficBeamTime(elapsed: number) {
    const uniform = this.trafficBeamMaterial.uniforms.beamTime;
    if (uniform && !reducedMotion?.matches) {
      uniform.value = elapsed;
    }
  }

  private updateDust(elapsed: number) {
    const uniform = this.dustMaterial.uniforms.dustTime;
    if (uniform && !reducedMotion?.matches) {
      uniform.value = elapsed;
    }
    if (reducedMotion?.matches) {
      return;
    }
    this.dustPoints.rotation.y = Math.sin(elapsed * 0.045) * 0.045;
    this.dustPoints.position.x = Math.sin(elapsed * 0.07) * 0.36;
    this.dustPoints.position.y = Math.sin(elapsed * 0.11) * 0.28;
    this.dustPoints.position.z = Math.cos(elapsed * 0.06) * 0.36;
  }

  private updatePostprocessingFocus() {
    const uniforms = this.bokehPass.uniforms as { focus?: { value: number }; aperture?: { value: number }; maxblur?: { value: number } };
    const targetDistance = this.currentFocusDistance();
    if (uniforms.focus) {
      uniforms.focus.value = THREE.MathUtils.lerp(uniforms.focus.value, targetDistance, 0.22);
    }
    if (uniforms.aperture) {
      uniforms.aperture.value = this.selectedItemId ? 0.00018 : 0.000075;
    }
    if (uniforms.maxblur) {
      uniforms.maxblur.value = this.selectedItemId ? 0.065 : 0.028;
    }
  }

  private currentFocusDistance() {
    const hoveredPoint = this.hovered ? this.worldFocusPointFor(this.hovered, this.focusPoint) : null;
    if (hoveredPoint) {
      return this.camera.position.distanceTo(hoveredPoint);
    }
    if (this.hasPointerFocus) {
      this.raycaster.setFromCamera(this.focusPointer, this.camera);
      const groundPoint = this.raycaster.ray.intersectPlane(this.focusPlane, this.focusPoint);
      if (groundPoint) {
        return this.camera.position.distanceTo(groundPoint);
      }
    }
    const selectedObject = this.selectedItemId ? this.objectsById.get(this.selectedItemId) : null;
    const selectedPoint = selectedObject ? this.worldFocusPointFor(selectedObject, this.focusPoint) : null;
    return this.camera.position.distanceTo(selectedPoint || this.controls.target);
  }

  private worldFocusPointFor(object: THREE.Object3D, target: THREE.Vector3) {
    object.getWorldPosition(target);
    const item = object.userData.item as MapLayoutItem | undefined;
    if (item) {
      target.y += Math.max(0.35, item.size * 0.36);
    }
    return target;
  }

  private onPointerDown = (event: PointerEvent) => {
    if (event.button !== 0) {
      this.pointerDown = null;
      this.pointerMovedSinceDown = true;
      return;
    }
    this.pointerDown = { x: event.clientX, y: event.clientY };
    this.pointerMovedSinceDown = false;
  };

  private onPointerMove = (event: PointerEvent) => {
    if (this.pointerDown) {
      const dx = event.clientX - this.pointerDown.x;
      const dy = event.clientY - this.pointerDown.y;
      if (Math.hypot(dx, dy) > clickSelectionMaxDistance) {
        this.pointerMovedSinceDown = true;
      }
    }
    const rect = this.renderer.domElement.getBoundingClientRect();
    const object = this.pickObjectAt(event.clientX, event.clientY);
    this.updatePointerFocus(event.clientX, event.clientY);
    this.setHovered(object, event.clientX - rect.left, event.clientY - rect.top);
  };

  private updatePointerFocus(clientX: number, clientY: number) {
    const rect = this.renderer.domElement.getBoundingClientRect();
    this.focusPointer.x = ((clientX - rect.left) / rect.width) * 2 - 1;
    this.focusPointer.y = -((clientY - rect.top) / rect.height) * 2 + 1;
    this.hasPointerFocus = true;
  }

  private pickObjectAt(clientX: number, clientY: number) {
    const rect = this.renderer.domElement.getBoundingClientRect();
    this.pointer.x = ((clientX - rect.left) / rect.width) * 2 - 1;
    this.pointer.y = -((clientY - rect.top) / rect.height) * 2 + 1;
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const hit = this.raycaster.intersectObjects(this.interactive, true)[0]?.object;
    return hit ? interactiveParent(hit) : null;
  }

  private onPointerUp = () => {
    this.pointerDown = null;
  };

  private onPointerCancel = () => {
    this.pointerDown = null;
    this.pointerMovedSinceDown = true;
  };

  private onPointerLeave = () => {
    if (this.pointerDown) {
      this.pointerMovedSinceDown = true;
    }
    this.hasPointerFocus = false;
    this.setHovered(null, 0, 0);
  };

  private onClick = (event: MouseEvent) => {
    if (this.pointerMovedSinceDown) {
      this.pointerMovedSinceDown = false;
      return;
    }
    const object = this.pickObjectAt(event.clientX, event.clientY);
    if (!object) {
      this.setHovered(null, 0, 0);
      this.clearSelection({ pushHistory: true });
      return;
    }
    const item = object.userData.item as MapLayoutItem | undefined;
    if (item) {
      this.selectItemById(item.id, { pushHistory: true, zoom: true });
    }
  };

  private onShellClick = (event: MouseEvent) => {
    const target = event.target instanceof Element ? event.target : null;
    const treeItem = target?.closest<HTMLElement>("[data-map-tree-id]");
    if (treeItem) {
      this.selectItemById(treeItem.dataset.mapTreeId || "", { pushHistory: true, zoom: true });
      return;
    }
    if (target?.closest("[data-map-reset]")) {
      this.resetCamera();
      return;
    }
    const toggle = target?.closest<HTMLElement>("[data-map-toggle]");
    if (toggle) {
      this.togglePaused(toggle);
      return;
    }
    const autorotate = target?.closest<HTMLElement>("[data-map-autorotate]");
    if (autorotate) {
      this.toggleAutorotate(autorotate);
    }
  };

  private onSelectionPopstate = (event: Event) => {
    const detail = (event as CustomEvent<{ selectedId?: string }>).detail;
    const selectedId = detail?.selectedId || "";
    if (!selectedId) {
      this.selectedItemId = null;
      this.pendingHistorySelectionId = null;
      this.shell.dataset.mapSelected = "";
      this.applyVisualState();
      this.renderSidebar();
      this.resetCamera();
      return;
    }
    this.selectItemById(selectedId, { zoom: true });
  };

  private resetCamera = (animate = true) => {
    this.focusTween = null;
    this.scene.rotation.y = 0;
    if (animate && !reducedMotion?.matches) {
      this.focusTween = {
        start: performance.now(),
        duration: 520,
        fromCamera: this.camera.position.clone(),
        toCamera: this.homeCameraPosition.clone(),
        fromTarget: this.controls.target.clone(),
        toTarget: this.homeTarget.clone(),
      };
      return;
    }
    this.camera.position.copy(this.homeCameraPosition);
    this.controls.target.copy(this.homeTarget);
    this.controls.update();
  };

  private togglePaused = (button: HTMLElement | null) => {
    this.paused = !this.paused;
    this.syncPauseButton(button);
  };

  private toggleAutorotate = (button: HTMLElement | null) => {
    if (reducedMotion?.matches) {
      this.autorotate = false;
      this.syncAutorotateButton(button);
      return;
    }
    this.autorotate = !this.autorotate;
    this.syncAutorotateButton(button);
  };

  private setHovered(object: THREE.Object3D | null, x: number, y: number) {
    if (this.hovered === object) {
      this.positionTooltip(x, y);
      return;
    }
    this.hovered = object;
    const item = object?.userData.item as MapLayoutItem | undefined;
    this.hoveredItemId = item?.id || null;
    this.applyVisualState();
    this.positionTooltip(x, y);
  }

  private selectItemById(id: string, options: { pushHistory?: boolean; zoom?: boolean } = {}) {
    const object = this.objectsById.get(id);
    const item = object?.userData.item as MapLayoutItem | undefined;
    if (!object || !item) {
      return;
    }
    this.selectedItemId = item.id;
    this.pendingHistorySelectionId = null;
    this.shell.dataset.mapSelected = this.selectedItemId || "";
    if (options.pushHistory) {
      writeMapSelectedToHistory(this.selectedItemId);
    }
    this.applyVisualState();
    this.renderSidebar(item);
    if (options.zoom) {
      this.zoomToObject(object, item);
    }
  }

  private clearSelection(options: { pushHistory?: boolean } = {}) {
    if (!this.selectedItemId) {
      return;
    }
    this.selectedItemId = null;
    this.pendingHistorySelectionId = null;
    this.shell.dataset.mapSelected = "";
    if (options.pushHistory) {
      writeMapSelectedToHistory(null);
    }
    this.applyVisualState();
    this.renderSidebar();
  }

  private restoreSelectedItem() {
    if (!this.selectedItemId) {
      this.selectedItemId = readMapSelectedFromURL();
      this.pendingHistorySelectionId = this.selectedItemId;
      this.shell.dataset.mapSelected = this.selectedItemId || "";
    }
    if (!this.selectedItemId) {
      this.applyVisualState();
      this.renderSidebar();
      return;
    }
    const object = this.objectsById.get(this.selectedItemId);
    if (!object) {
      this.selectedItemId = null;
      this.pendingHistorySelectionId = null;
      this.shell.dataset.mapSelected = "";
      this.applyVisualState();
      this.renderSidebar();
      return;
    }
    const item = object.userData.item as MapLayoutItem | undefined;
    this.applyVisualState();
    if (item) {
      this.renderSidebar(item);
      if (this.pendingHistorySelectionId === item.id) {
        this.zoomToObject(object, item);
        this.pendingHistorySelectionId = null;
      }
    }
  }

  private applyVisualState() {
    const item = this.selectedItemId ? (this.objectsById.get(this.selectedItemId)?.userData.item as MapLayoutItem | undefined) : undefined;
    const related = item ? this.relatedConnectorIds(item) : new Set<string>();
    for (const object of this.interactive) {
      const objectItem = object.userData.item as MapLayoutItem | undefined;
      if (!objectItem) {
        continue;
      }
      const selected = objectItem.id === this.selectedItemId;
      const hovered = objectItem.id === this.hoveredItemId;
      const relatedToSelection = related.has(objectItem.id) && objectItem.id !== item?.id;
      setObjectVisualState(object, {
        highlighted: selected || hovered || relatedToSelection,
        scale: selected ? 1.14 : hovered ? 1.08 : relatedToSelection ? 1.05 : 1,
      });
    }
    for (const line of this.connectors) {
      line.visible = true;
    }
    this.updateSelectionEffectTarget(item);
    this.particleMaterial.uniforms.sparkOpacity.value = item ? 1.02 : 0.94;
  }

  private relatedConnectorIds(item: MapLayoutItem) {
    const ids = new Set<string>([item.id, ...item.ownedIds, ...item.targetIds, ...item.trafficTargetIds]);
    for (const id of [...item.ownedIds, ...item.targetIds, ...item.trafficTargetIds]) {
      const child = this.objectsById.get(id)?.userData.item as MapLayoutItem | undefined;
      if (!child) {
        continue;
      }
      ids.add(child.id);
      child.ownedIds.forEach((childId) => ids.add(childId));
      child.targetIds.forEach((targetId) => ids.add(targetId));
      child.trafficTargetIds.forEach((targetId) => ids.add(targetId));
    }
    for (const object of this.interactive) {
      const candidate = object.userData.item as MapLayoutItem | undefined;
      if (!candidate) {
        continue;
      }
      if (candidate.trafficTargetIds.some((targetId) => ids.has(targetId) || targetId === item.id)) {
        ids.add(candidate.id);
      }
      if (candidate.type === "warning" && candidate.targetIds.some((targetId) => ids.has(targetId))) {
        ids.add(candidate.id);
      }
    }
    return ids;
  }

  private updateSelectionEffectTarget(item?: MapLayoutItem) {
    if (this.selectionEffect) {
      this.selectionEffect.removeFromParent();
      disposeObject(this.selectionEffect);
    }
    this.selectionEffect = null;
    if (!item) {
      return;
    }
    const object = this.objectsById.get(item.id);
    if (!object) {
      return;
    }
    const position = new THREE.Vector3();
    object.getWorldPosition(position);
    const effect = selectionEffectFor(item);
    effect.position.copy(position);
    this.selectionEffect = effect;
    this.scene.add(effect);
  }

  private zoomToObject(object: THREE.Object3D, item: MapLayoutItem) {
    const target = new THREE.Vector3();
    object.getWorldPosition(target);
    target.y += Math.max(0.8, item.size * 0.42);
    const currentDirection = this.camera.position.clone().sub(this.controls.target);
    if (currentDirection.lengthSq() < 0.01) {
      currentDirection.set(18, 14, 18);
    }
    currentDirection.normalize();
    const distance = zoomDistanceFor(item);
    const desiredCamera = target.clone().add(currentDirection.multiplyScalar(distance));
    desiredCamera.y = Math.max(desiredCamera.y, target.y + 4);
    this.focusTween = {
      start: performance.now(),
      duration: reducedMotion?.matches ? 1 : 720,
      fromCamera: this.camera.position.clone(),
      toCamera: desiredCamera,
      fromTarget: this.controls.target.clone(),
      toTarget: target,
    };
  }

  private updateFocusTween() {
    if (!this.focusTween) {
      return;
    }
    const raw = Math.min(1, (performance.now() - this.focusTween.start) / this.focusTween.duration);
    const t = easeOutCubic(raw);
    this.camera.position.lerpVectors(this.focusTween.fromCamera, this.focusTween.toCamera, t);
    this.controls.target.lerpVectors(this.focusTween.fromTarget, this.focusTween.toTarget, t);
    if (raw >= 1) {
      this.focusTween = null;
    }
  }

  private rebuildParticleFlow() {
    this.particleGeometry.setAttribute("position", new THREE.BufferAttribute(new Float32Array(0), 3));
    this.particleGeometry.boundingSphere = new THREE.Sphere(new THREE.Vector3(0, 0, 0), 10000);
  }

  private updateParticleFlowTime(elapsed: number) {
    if (reducedMotion?.matches) {
      return;
    }
    this.particleMaterial.uniforms.sparkTime.value = elapsed;
  }

  private positionTooltip(x: number, y: number) {
    if (!this.tooltip) {
      return;
    }
    const item = this.hovered?.userData.item as MapLayoutItem | undefined;
    if (!item) {
      this.tooltip.hidden = true;
      return;
    }
    this.tooltip.hidden = false;
    this.tooltip.textContent = `${item.label} · ${item.detail}`;
    this.tooltip.style.transform = `translate(${Math.round(x + 14)}px, ${Math.round(y + 14)}px)`;
  }

  private clearTopology() {
    for (const collection of this.instancedCollections.values()) {
      this.scene.remove(collection.mesh);
      disposeObject(collection.mesh);
    }
    this.instancedCollections.clear();
    for (const object of [...this.interactive]) {
      this.scene.remove(object);
      disposeObject(object);
    }
    for (const child of [...this.scene.children]) {
      if (child.userData.connector) {
        this.scene.remove(child);
        disposeObject(child);
      }
    }
    this.interactive = [];
    this.objectsById.clear();
    this.connectors = [];
    this.labelLayer.replaceChildren();
    this.labelsById.clear();
    this.currentLayout = [];
    this.rebuildParticleFlow();
    if (this.selectionEffect) {
      this.selectionEffect.removeFromParent();
      disposeObject(this.selectionEffect);
    }
    this.selectionEffect = null;
    this.pulses = [];
    this.hovered = null;
    this.hoveredItemId = null;
    this.focusTween = null;
    if (!this.selectedItemId) {
      this.particleMaterial.uniforms.sparkOpacity.value = 0.94;
    }
  }

  private disposeScene() {
    for (const child of [...this.scene.children]) {
      this.scene.remove(child);
      disposeObject(child);
    }
  }

  private updateHomeView(layout: MapLayoutItem[]) {
    if (layout.length === 0) {
      this.homeCameraPosition.set(24, 22, 28);
      this.homeTarget.set(0, 0, 0);
      return;
    }
    const bounds = layout.reduce(
      (acc, item) => ({
        minX: Math.min(acc.minX, item.x - item.size),
        maxX: Math.max(acc.maxX, item.x + item.size),
        minZ: Math.min(acc.minZ, item.z - item.size),
        maxZ: Math.max(acc.maxZ, item.z + item.size),
      }),
      { minX: Infinity, maxX: -Infinity, minZ: Infinity, maxZ: -Infinity },
    );
    const centerX = (bounds.minX + bounds.maxX) / 2;
    const centerZ = (bounds.minZ + bounds.maxZ) / 2;
    const spanX = bounds.maxX - bounds.minX;
    const spanZ = bounds.maxZ - bounds.minZ;
    const distance = Math.max(34, Math.max(spanX, spanZ) * 0.78);
    this.homeTarget.set(centerX, 0, centerZ);
    this.homeCameraPosition.set(centerX + distance * 0.12, Math.max(48, distance * 1.08), centerZ + distance * 0.18);
    this.controls.minDistance = Math.max(7, Math.min(18, distance * 0.11));
    this.controls.maxDistance = Math.max(90, distance * 2.5);
  }

  private updateLabels() {
    const width = this.viewport.clientWidth;
    const height = this.viewport.clientHeight;
    if (width <= 0 || height <= 0 || this.labelsById.size === 0) {
      return;
    }
    const cameraDistance = this.camera.position.distanceTo(this.controls.target);
    const candidates: Array<{
      id: string;
      element: HTMLElement;
      priority: number;
      x: number;
      y: number;
      forced: boolean;
    }> = [];
    const world = new THREE.Vector3();
    const projected = new THREE.Vector3();

    for (const object of this.interactive) {
      const item = object.userData.item as MapLayoutItem | undefined;
      const element = item ? this.labelsById.get(item.id) : undefined;
      if (!item || !element) {
        continue;
      }
      object.getWorldPosition(world);
      world.y += Math.max(0.85, item.size * 0.62);
      projected.copy(world).project(this.camera);
      if (projected.z < -1 || projected.z > 1) {
        element.hidden = true;
        continue;
      }
      const forced = item.id === this.selectedItemId || item.id === this.hoveredItemId;
      if (!forced && !labelVisibleAtDistance(item, cameraDistance)) {
        element.hidden = true;
        continue;
      }
      candidates.push({
        id: item.id,
        element,
        priority: labelPriority(item, forced) - this.camera.position.distanceTo(world) * 0.012,
        x: (projected.x * 0.5 + 0.5) * width,
        y: (-projected.y * 0.5 + 0.5) * height,
        forced,
      });
    }

    const visibleIds = new Set<string>();
    const budget = labelBudget(cameraDistance);
    for (const candidate of candidates.sort((left, right) => right.priority - left.priority)) {
      if (!candidate.forced && visibleIds.size >= budget) {
        candidate.element.hidden = true;
        continue;
      }
      candidate.element.hidden = false;
      candidate.element.classList.toggle("is-selected", candidate.id === this.selectedItemId);
      candidate.element.classList.toggle("is-hovered", candidate.id === this.hoveredItemId);
      candidate.element.style.transform = `translate3d(${Math.round(candidate.x)}px, ${Math.round(candidate.y)}px, 0) translate(-50%, calc(-100% - 8px))`;
      visibleIds.add(candidate.id);
    }
    for (const [id, element] of this.labelsById) {
      if (!visibleIds.has(id)) {
        element.hidden = true;
      }
    }
  }
}

function initClusterMaps() {
  for (const shell of document.querySelectorAll<HTMLElement>("[data-cluster-map]")) {
    const controller = controllers.get(shell);
    if (controller) {
      controller.syncShell();
      continue;
    }
    try {
      controllers.set(shell, new ClusterMapController(shell));
    } catch (error) {
      console.error("initialize cluster map", error);
    }
  }
}

function cleanupClusterMaps() {
  for (const [shell, controller] of Array.from(controllers.entries())) {
    if (!document.body.contains(shell)) {
      controller.destroy();
      controllers.delete(shell);
    }
  }
}

function installMapDebugHook() {
  (window as MapDebugWindow).__kubernettoMapDebug = () => Array.from(controllers.values()).map((controller) => controller.debugStats());
}

function sceneDrawableStats(scene: THREE.Scene) {
  const stats = {
    total: 0,
    instancedMeshes: 0,
    meshes: 0,
    lineSegments: 0,
    points: 0,
  };
  scene.traverse((object) => {
    if (!object.visible) {
      return;
    }
    const maybeInstanced = object as THREE.InstancedMesh & { isInstancedMesh?: boolean };
    const maybeMesh = object as THREE.Mesh & { isMesh?: boolean };
    const maybeLineSegments = object as THREE.LineSegments & { isLineSegments?: boolean };
    const maybePoints = object as THREE.Points & { isPoints?: boolean };
    if (maybeInstanced.isInstancedMesh) {
      stats.instancedMeshes += 1;
      stats.total += 1;
    } else if (maybeMesh.isMesh) {
      stats.meshes += 1;
      stats.total += 1;
    } else if (maybeLineSegments.isLineSegments) {
      stats.lineSegments += 1;
      stats.total += 1;
    } else if (maybePoints.isPoints) {
      stats.points += 1;
      stats.total += 1;
    }
  });
  return stats;
}

function readMapSelectedFromURL() {
  return new URLSearchParams(window.location.search).get("mapSelected") || "";
}

function writeMapSelectedToHistory(selectedId: string | null) {
  if (!window.history.pushState) {
    return;
  }
  const url = new URL(window.location.href);
  if (selectedId) {
    url.searchParams.set("mapSelected", selectedId);
  } else {
    url.searchParams.delete("mapSelected");
  }
  const nextURL = url.pathname + url.search + url.hash;
  if (nextURL === window.location.pathname + window.location.search + window.location.hash) {
    window.history.replaceState({ ...window.history.state, mapSelected: selectedId || "" }, "", nextURL);
    return;
  }
  window.history.pushState({ ...window.history.state, mapSelected: selectedId || "" }, "", nextURL);
}

function connectorGeometries(from: THREE.Vector3, to: THREE.Vector3, seedKey: string) {
  const midpoint = from.clone().lerp(to, 0.5);
  const distance = from.distanceTo(to);
  midpoint.y += Math.min(5.8, Math.max(1.2, distance * 0.12));
  const curve = new THREE.QuadraticBezierCurve3(from.clone(), midpoint, to.clone());
  return {
    beam: createBeamGeometry(curve, seedKey, distance),
    branches: createBeamSparkGeometry(curve, seedKey, distance),
  };
}

function createBeamGeometry(curve: THREE.QuadraticBezierCurve3, seedKey: string, distance: number) {
  const segmentCount = 34;
  const radialSegments = 6;
  const vertexCount = (segmentCount + 1) * radialSegments;
  const positions = new Float32Array(vertexCount * 3);
  const progress = new Float32Array(vertexCount);
  const seeds = new Float32Array(vertexCount);
  const branches = new Float32Array(vertexCount);
  const edges = new Float32Array(vertexCount);
  const widths = new Float32Array(vertexCount);
  const sideVectors = new Float32Array(vertexCount * 3);
  const upVectors = new Float32Array(vertexCount * 3);
  const radials = new Float32Array(vertexCount * 2);
  const facets = new Float32Array(vertexCount);
  const indices = new Uint16Array(segmentCount * radialSegments * 6);
  const seed = seededUnitValue(seedKey);
  const jitter = Math.min(0.42, Math.max(0.08, distance * 0.018));
  const width = Math.min(0.18, Math.max(0.064, distance * 0.0062));
  for (let index = 0; index <= segmentCount; index += 1) {
    const t = index / segmentCount;
    const point = curve.getPoint(t);
    if (index > 0 && index < segmentCount) {
      point.add(beamJitter(curve, t, `${seedKey}:main:${index}`, jitter));
    }
    const tangent = curve.getTangent(t).normalize();
    const side = beamSideVector(tangent);
    const up = new THREE.Vector3().crossVectors(side, tangent).normalize();
    const twist = t * Math.PI * 2.6 + seed * Math.PI * 2;
    for (let radialIndex = 0; radialIndex < radialSegments; radialIndex += 1) {
      const angle = (radialIndex / radialSegments) * Math.PI * 2 + twist;
      const vertexIndex = index * radialSegments + radialIndex;
      writeBeamVertex(positions, progress, seeds, branches, edges, widths, sideVectors, upVectors, radials, facets, vertexIndex, {
        point,
        t,
        seed,
        branch: 0,
        edge: 0,
        width,
        side,
        up,
        radialX: Math.cos(angle),
        radialY: Math.sin(angle),
        facet: radialIndex % 2 === 0 ? 1 : 0.38,
      });
    }
    if (index < segmentCount) {
      for (let radialIndex = 0; radialIndex < radialSegments; radialIndex += 1) {
        const nextRadial = (radialIndex + 1) % radialSegments;
        const indexOffset = (index * radialSegments + radialIndex) * 6;
        const current = index * radialSegments + radialIndex;
        const currentNext = index * radialSegments + nextRadial;
        const next = (index + 1) * radialSegments + radialIndex;
        const nextNext = (index + 1) * radialSegments + nextRadial;
        indices[indexOffset] = current;
        indices[indexOffset + 1] = currentNext;
        indices[indexOffset + 2] = next;
        indices[indexOffset + 3] = currentNext;
        indices[indexOffset + 4] = nextNext;
        indices[indexOffset + 5] = next;
      }
    }
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setIndex(new THREE.BufferAttribute(indices, 1));
  geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute("beamU", new THREE.BufferAttribute(progress, 1));
  geometry.setAttribute("beamSeed", new THREE.BufferAttribute(seeds, 1));
  geometry.setAttribute("beamBranch", new THREE.BufferAttribute(branches, 1));
  geometry.setAttribute("beamEdge", new THREE.BufferAttribute(edges, 1));
  geometry.setAttribute("beamWidth", new THREE.BufferAttribute(widths, 1));
  geometry.setAttribute("beamSideVector", new THREE.BufferAttribute(sideVectors, 3));
  geometry.setAttribute("beamUpVector", new THREE.BufferAttribute(upVectors, 3));
  geometry.setAttribute("beamRadial", new THREE.BufferAttribute(radials, 2));
  geometry.setAttribute("beamFacet", new THREE.BufferAttribute(facets, 1));
  geometry.boundingSphere = new THREE.Sphere(curve.getPoint(0.5), distance * 0.72 + 8);
  return geometry;
}

function createBeamSparkGeometry(curve: THREE.QuadraticBezierCurve3, seedKey: string, distance: number) {
  const sparkCount = Math.min(6, Math.max(2, Math.round(distance / 8)));
  const vertexCount = sparkCount * 2;
  const positions = new Float32Array(vertexCount * 3);
  const progress = new Float32Array(vertexCount);
  const seeds = new Float32Array(vertexCount);
  const branches = new Float32Array(vertexCount);
  const edges = new Float32Array(vertexCount);
  const widths = new Float32Array(vertexCount);
  const sideVectors = new Float32Array(vertexCount * 3);
  const upVectors = new Float32Array(vertexCount * 3);
  const radials = new Float32Array(vertexCount * 2);
  const facets = new Float32Array(vertexCount);
  const seed = seededUnitValue(seedKey);
  const length = Math.min(1.15, Math.max(0.32, distance * 0.045));
  for (let index = 0; index < sparkCount; index += 1) {
    const t = 0.12 + seededUnitValue(`${seedKey}:spark-t:${index}`) * 0.76;
    const base = curve.getPoint(t).add(beamJitter(curve, t, `${seedKey}:spark-base:${index}`, length * 0.16));
    const tangent = curve.getTangent(t).normalize();
    const side = beamSideVector(tangent);
    const up = new THREE.Vector3(0, 1, 0);
    const sign = seededUnitValue(`${seedKey}:spark-sign:${index}`) > 0.5 ? 1 : -1;
    const tip = base
      .clone()
      .add(side.multiplyScalar(sign * length * (0.45 + seededUnitValue(`${seedKey}:spark-side:${index}`))))
      .add(up.multiplyScalar((seededUnitValue(`${seedKey}:spark-up:${index}`) - 0.2) * length * 0.56))
      .add(tangent.multiplyScalar((seededUnitValue(`${seedKey}:spark-forward:${index}`) - 0.5) * length * 0.3));
    const sparkSide = beamSideVector(tangent);
    const sparkUp = new THREE.Vector3().crossVectors(sparkSide, tangent).normalize();
    writeBeamVertex(positions, progress, seeds, branches, edges, widths, sideVectors, upVectors, radials, facets, index * 2, {
      point: base,
      t,
      seed,
      branch: 1,
      edge: 0,
      width: length * 0.09,
      side: sparkSide,
      up: sparkUp,
      radialX: 0,
      radialY: 0,
      facet: 1,
    });
    writeBeamVertex(positions, progress, seeds, branches, edges, widths, sideVectors, upVectors, radials, facets, index * 2 + 1, {
      point: tip,
      t: Math.min(1, t + 0.035),
      seed,
      branch: 1,
      edge: 0,
      width: length * 0.07,
      side: sparkSide,
      up: sparkUp,
      radialX: 0,
      radialY: 0,
      facet: 1,
    });
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute("beamU", new THREE.BufferAttribute(progress, 1));
  geometry.setAttribute("beamSeed", new THREE.BufferAttribute(seeds, 1));
  geometry.setAttribute("beamBranch", new THREE.BufferAttribute(branches, 1));
  geometry.setAttribute("beamEdge", new THREE.BufferAttribute(edges, 1));
  geometry.setAttribute("beamWidth", new THREE.BufferAttribute(widths, 1));
  geometry.setAttribute("beamSideVector", new THREE.BufferAttribute(sideVectors, 3));
  geometry.setAttribute("beamUpVector", new THREE.BufferAttribute(upVectors, 3));
  geometry.setAttribute("beamRadial", new THREE.BufferAttribute(radials, 2));
  geometry.setAttribute("beamFacet", new THREE.BufferAttribute(facets, 1));
  geometry.boundingSphere = new THREE.Sphere(curve.getPoint(0.5), distance * 0.72 + 8);
  return geometry;
}

function writeBeamVertex(
  positions: Float32Array,
  progress: Float32Array,
  seeds: Float32Array,
  branches: Float32Array,
  edges: Float32Array,
  widths: Float32Array,
  sideVectors: Float32Array,
  upVectors: Float32Array,
  radials: Float32Array,
  facets: Float32Array,
  index: number,
  {
    point,
    t,
    seed,
    branch,
    edge,
    width,
    side,
    up,
    radialX,
    radialY,
    facet,
  }: {
    point: THREE.Vector3;
    t: number;
    seed: number;
    branch: number;
    edge: number;
    width: number;
    side: THREE.Vector3;
    up: THREE.Vector3;
    radialX: number;
    radialY: number;
    facet: number;
  },
) {
  positions[index * 3] = point.x;
  positions[index * 3 + 1] = point.y;
  positions[index * 3 + 2] = point.z;
  progress[index] = t;
  seeds[index] = seed;
  branches[index] = branch;
  edges[index] = edge;
  widths[index] = width;
  sideVectors[index * 3] = side.x;
  sideVectors[index * 3 + 1] = side.y;
  sideVectors[index * 3 + 2] = side.z;
  upVectors[index * 3] = up.x;
  upVectors[index * 3 + 1] = up.y;
  upVectors[index * 3 + 2] = up.z;
  radials[index * 2] = radialX;
  radials[index * 2 + 1] = radialY;
  facets[index] = facet;
}

function beamJitter(curve: THREE.QuadraticBezierCurve3, t: number, key: string, amount: number) {
  const tangent = curve.getTangent(t).normalize();
  const side = beamSideVector(tangent);
  return side
    .multiplyScalar((seededUnitValue(`${key}:side`) - 0.5) * amount * 2)
    .add(new THREE.Vector3(0, (seededUnitValue(`${key}:up`) - 0.5) * amount * 0.95, 0));
}

function beamSideVector(tangent: THREE.Vector3) {
  const side = new THREE.Vector3().crossVectors(tangent, new THREE.Vector3(0, 1, 0));
  if (side.lengthSq() < 0.001) {
    side.crossVectors(tangent, new THREE.Vector3(1, 0, 0));
  }
  return side.normalize();
}

function labelForItem(item: MapLayoutItem) {
  const label = document.createElement("div");
  label.className = `map-label map-label-${item.type}`;
  label.textContent = item.label;
  label.title = `${item.label} · ${item.detail}`;
  label.hidden = true;
  return label;
}

function treeKindLabel(item: MapLayoutItem) {
  switch (item.type) {
    case "namespace":
      return "NS";
    case "category":
      return "Group";
    case "clusterResource":
      return item.detail.split(" · ")[0] || "Cluster";
    case "workload":
      return item.detail.split(" · ")[0] || "Workload";
    case "service":
      return "Svc";
    case "warning":
      return "Warn";
    case "node":
      return "Node";
    case "pod":
    default:
      return "Pod";
  }
}

function treeKindIconClass(item: MapLayoutItem) {
  switch (item.type) {
    case "namespace":
      return "icon-[uil--folder-network]";
    case "category":
      return "icon-[uil--layer-group]";
    case "clusterResource":
      return resourceKindIconClass(item.detail.split(" · ")[0]);
    case "workload":
      return resourceKindIconClass(item.detail.split(" · ")[0]);
    case "service":
      return "icon-[uil--server-alt]";
    case "warning":
      return "icon-[uil--bolt]";
    case "node":
      return "icon-[uil--server-network]";
    case "pod":
    default:
      return "icon-[uil--cube]";
  }
}

function resourceKindIconClass(kind: string) {
  switch (kind.toLowerCase()) {
    case "deployments":
    case "deployment":
      return "icon-[uil--rocket]";
    case "statefulsets":
    case "statefulset":
      return "icon-[uil--layers]";
    case "daemonsets":
    case "daemonset":
      return "icon-[uil--layer-group]";
    case "replicasets":
    case "replicaset":
      return "icon-[uil--copy]";
    case "jobs":
    case "job":
      return "icon-[uil--check-circle]";
    case "cronjobs":
    case "cronjob":
      return "icon-[uil--clock]";
    case "persistentvolumes":
    case "persistentvolume":
      return "icon-[uil--server]";
    case "persistentvolumeclaims":
    case "persistentvolumeclaim":
      return "icon-[uil--database]";
    case "storageclasses":
    case "storageclass":
      return "icon-[uil--archive]";
    case "roles":
    case "role":
      return "icon-[uil--key-skeleton]";
    case "rolebindings":
    case "rolebinding":
      return "icon-[uil--link]";
    case "clusterroles":
    case "clusterrole":
      return "icon-[uil--keyhole-circle]";
    case "clusterrolebindings":
    case "clusterrolebinding":
      return "icon-[uil--link-h]";
    case "priorityclasses":
    case "priorityclass":
      return "icon-[uil--arrow-up]";
    case "runtimeclasses":
    case "runtimeclass":
      return "icon-[uil--processor]";
    case "mutatingwebhookconfigurations":
    case "validatingwebhookconfigurations":
      return "icon-[uil--web-grid]";
    default:
      return "icon-[uil--servers]";
  }
}

function statusLabel(statusKey: string) {
  switch (statusKey) {
    case "danger":
      return "Error";
    case "warn":
      return "Warn";
    case "good":
      return "Ready";
    default:
      return "OK";
  }
}

function serviceHasNoTargets(item: MapLayoutItem) {
  return item.type === "service" && item.detail.includes("0 targets");
}

function compareInsightItems(left: MapLayoutItem, right: MapLayoutItem) {
  const statusDelta = insightStatusRank(left) - insightStatusRank(right);
  if (statusDelta !== 0) {
    return statusDelta;
  }
  const typeDelta = insightTypeRank(left.type) - insightTypeRank(right.type);
  if (typeDelta !== 0) {
    return typeDelta;
  }
  return left.label.localeCompare(right.label);
}

function insightStatusRank(item: MapLayoutItem) {
  if (item.statusKey === "danger") {
    return 0;
  }
  if (item.statusKey === "warn") {
    return 1;
  }
  if (serviceHasNoTargets(item)) {
    return 2;
  }
  if (item.statusKey === "good") {
    return 4;
  }
  return 3;
}

function insightTypeRank(type: MapItemType) {
  switch (type) {
    case "warning":
      return 0;
    case "namespace":
      return 1;
    case "workload":
      return 2;
    case "service":
      return 3;
    case "pod":
      return 4;
    case "node":
      return 5;
    case "category":
      return 6;
    case "clusterResource":
    default:
      return 7;
  }
}

function labelBudget(cameraDistance: number) {
  if (cameraDistance < 28) {
    return 90;
  }
  if (cameraDistance < 45) {
    return 62;
  }
  if (cameraDistance < 70) {
    return 42;
  }
  if (cameraDistance < 95) {
    return 28;
  }
  return 16;
}

function labelVisibleAtDistance(item: MapLayoutItem, cameraDistance: number) {
  switch (item.type) {
    case "category":
    case "namespace":
      return cameraDistance < 140;
    case "warning":
      return cameraDistance < 120;
    case "node":
      return cameraDistance < 105;
    case "workload":
    case "service":
      return cameraDistance < 74;
    case "clusterResource":
      return cameraDistance < 54;
    case "pod":
    default:
      return cameraDistance < 42;
  }
}

function labelPriority(item: MapLayoutItem, forced: boolean) {
  if (forced) {
    return 1000;
  }
  const statusBoost = item.statusKey === "danger" ? 120 : item.statusKey === "warn" ? 60 : 0;
  switch (item.type) {
    case "warning":
      return 760 + statusBoost;
    case "category":
      return 700 + statusBoost;
    case "namespace":
      return 660 + statusBoost;
    case "node":
      return 610 + statusBoost;
    case "workload":
      return 540 + statusBoost;
    case "service":
      return 520 + statusBoost;
    case "clusterResource":
      return 410 + statusBoost;
    case "pod":
    default:
      return 360 + statusBoost;
  }
}

function solidMaterial(color: number, opacity: number) {
  return new THREE.MeshStandardMaterial({
    color,
    emissive: color,
    emissiveIntensity: 0.18,
    flatShading: true,
    metalness: 0.15,
    roughness: 0.48,
    transparent: opacity < 1,
    depthWrite: opacity >= 1,
    opacity,
  });
}

function wireMaterialFor(color: number) {
  return new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.9 });
}

function instancedPrimitiveType(item: MapLayoutItem): InstancedPrimitiveType | null {
  switch (item.type) {
    case "clusterResource":
    case "node":
    case "pod":
    case "service":
    case "warning":
      return item.type;
    default:
      return null;
  }
}

function instancedGeometryFor(type: InstancedPrimitiveType) {
  switch (type) {
    case "clusterResource":
      return new THREE.DodecahedronGeometry(1, 0);
    case "node":
      return new THREE.BoxGeometry(1, 1, 1);
    case "service":
      return new THREE.OctahedronGeometry(1, 1);
    case "warning":
      return new THREE.IcosahedronGeometry(1, 2);
    case "pod":
    default:
      return new THREE.SphereGeometry(1, 18, 14);
  }
}

function instancedMaterialFor(type: InstancedPrimitiveType) {
  const material = new THREE.MeshStandardMaterial({
    color: 0xffffff,
    emissive: type === "warning" ? 0x8a1111 : 0x061412,
    emissiveIntensity: type === "warning" ? 0.74 : 0.14,
    flatShading: true,
    metalness: type === "warning" ? 0.34 : 0.15,
    roughness: type === "warning" ? 0.22 : 0.48,
    vertexColors: true,
  });
  if (type === "warning") {
    material.onBeforeCompile = (shader) => {
      shader.uniforms.warningTime = { value: 0 };
      shader.vertexShader = shader.vertexShader
        .replace(
          "#include <common>",
          `#include <common>
uniform float warningTime;
attribute float warningSeed;
varying float vWarningSeed;`,
        )
        .replace(
          "#include <begin_vertex>",
          `vec3 transformed = vec3(position);
float warningSeedValue = warningSeed;
vWarningSeed = warningSeedValue;
${warningDeformSnippet}`,
        );
      shader.fragmentShader = shader.fragmentShader
        .replace(
          "#include <common>",
          `#include <common>
uniform float warningTime;
varying float vWarningSeed;`,
        )
        .replace(
          "vec3 outgoingLight = totalDiffuse + totalSpecular + totalEmissiveRadiance;",
          `float warningRim = pow(1.0 - abs(dot(normalize(normal), normalize(vViewPosition))), 3.2);
float warningPhase = vWarningSeed * 37.17;
float warningSweep = smoothstep(0.86, 1.0, sin(warningTime * (6.2 + vWarningSeed * 2.4) + warningPhase + vViewPosition.x * 3.4 - vViewPosition.y * 5.1) * 0.5 + 0.5);
float warningFacet = pow(max(dot(normalize(normal), normalize(vec3(0.28 + vWarningSeed * 0.18, 0.82, 0.48 - vWarningSeed * 0.15))), 0.0), 18.0);
vec3 warningGlint = vec3(1.0, 0.86, 0.62) * (warningFacet * 0.85 + warningRim * warningSweep * 0.46);
vec3 outgoingLight = totalDiffuse + totalSpecular + totalEmissiveRadiance + warningGlint;`,
        );
      material.userData.warningShader = shader;
    };
  }
  return material;
}

function instancedBaseScaleFor(item: MapLayoutItem) {
  if (item.type === "node") {
    return new THREE.Vector3(1.55, Math.max(2.6, item.size), 1.55);
  }
  return new THREE.Vector3(item.size, item.size, item.size);
}

function writeInstancedInstance(instance: InstancedInstance, scale: THREE.Vector3, color = instance.baseColor, position = instance.position) {
  instance.position.copy(position);
  instancedMatrix.compose(instance.position, identityQuaternion, scale);
  instance.mesh.setMatrixAt(instance.index, instancedMatrix);
  instance.mesh.setColorAt(instance.index, color);
  instance.mesh.instanceMatrix.needsUpdate = true;
  if (instance.mesh.instanceColor) {
    instance.mesh.instanceColor.needsUpdate = true;
  }
}

function seededUnitValue(value: string) {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return ((hash >>> 0) % 10000) / 10000;
}

function createDustGeometry() {
  const geometry = new THREE.BufferGeometry();
  const positions = new Float32Array(dustParticleCount * 3);
  const seeds = new Float32Array(dustParticleCount);
  const sizes = new Float32Array(dustParticleCount);
  for (let index = 0; index < dustParticleCount; index += 1) {
    const seed = seededUnitValue(`dust:${index}`);
    const radius = 18 + seededUnitValue(`dust-radius:${index}`) * 92;
    const angle = seed * Math.PI * 2;
    positions[index * 3] = Math.cos(angle) * radius + (seededUnitValue(`dust-x:${index}`) - 0.5) * 22;
    positions[index * 3 + 1] = 1.8 + seededUnitValue(`dust-y:${index}`) * 28;
    positions[index * 3 + 2] = Math.sin(angle) * radius + (seededUnitValue(`dust-z:${index}`) - 0.5) * 22;
    seeds[index] = seed;
    sizes[index] = 1.1 + seededUnitValue(`dust-size:${index}`) * 2.6;
  }
  geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute("dustSeed", new THREE.BufferAttribute(seeds, 1));
  geometry.setAttribute("dustSize", new THREE.BufferAttribute(sizes, 1));
  geometry.boundingSphere = new THREE.Sphere(new THREE.Vector3(0, 12, 0), 130);
  return geometry;
}

function selectionEffectFor(item: MapLayoutItem) {
  const color = colorFor(item.statusKey, item.type);
  const group = new THREE.Group();
  group.add(...selectionSpotlightFor(item));
  if (item.type === "warning") {
    const scale = instancedBaseScaleFor(item);
    const glow = new THREE.Mesh(instancedGeometryFor("warning"), warningSelectionMaterial(item, color, false));
    glow.scale.copy(scale).multiplyScalar(1.015);
    glow.renderOrder = 96;
    const outline = new THREE.Mesh(instancedGeometryFor("warning"), warningSelectionMaterial(item, 0xfff1c4, true));
    outline.scale.copy(scale).multiplyScalar(1.022);
    outline.renderOrder = 98;
    group.add(glow, outline);
    group.userData.selectionEffect = true;
    return group;
  }
  const wireMaterial = new THREE.LineBasicMaterial({
    color: 0x9fffe8,
    transparent: true,
    opacity: 0.92,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  const glowMaterial = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 0.1,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    side: THREE.DoubleSide,
  });
  const geometry = selectionEffectGeometry(item);
  const outline = new THREE.LineSegments(new THREE.EdgesGeometry(geometry), wireMaterial);
  outline.renderOrder = 97;
  const glow = selectionGlowFor(item, geometry, glowMaterial);
  if (glow) {
    glow.renderOrder = 96;
    group.add(glow);
  } else {
    geometry.dispose();
  }
  group.add(outline);
  group.userData.selectionEffect = true;
  return group;
}

function selectionSpotlightFor(item: MapLayoutItem) {
  const broad = item.type === "namespace" || item.type === "category";
  const haloRadius = broad ? item.size * 1.18 : THREE.MathUtils.clamp(item.size * 1.65, 1.8, 6.2);
  const effects: THREE.Object3D[] = [];
  const haloGeometry = new THREE.CircleGeometry(haloRadius, 64);
  const halo = new THREE.Mesh(haloGeometry, spotlightHaloMaterial());
  halo.rotation.x = -Math.PI / 2;
  halo.position.y = 0.035;
  halo.renderOrder = 96;
  halo.userData.excludeFromBokehDepth = true;
  effects.push(halo);
  return effects;
}

function spotlightHaloMaterial() {
  const material = new THREE.ShaderMaterial({
    uniforms: {
      spotlightTime: { value: 0 },
    },
    vertexShader: `
      varying vec2 vSpotUv;
      void main() {
        vSpotUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      uniform float spotlightTime;
      varying vec2 vSpotUv;
      void main() {
        vec2 point = vSpotUv - vec2(0.5);
        float radial = length(point) * 2.0;
        float core = 1.0 - smoothstep(0.0, 0.46, radial);
        float ring = 1.0 - smoothstep(0.68, 1.0, radial);
        float pulse = sin(spotlightTime * 1.8) * 0.018;
        float alpha = (core * 0.13 + ring * 0.07 + pulse) * (1.0 - smoothstep(0.92, 1.0, radial));
        gl_FragColor = vec4(0.72, 1.0, 0.9, max(alpha, 0.0));
      }
    `,
    transparent: true,
    depthTest: false,
    depthWrite: false,
    side: THREE.DoubleSide,
    blending: THREE.AdditiveBlending,
  });
  material.userData.spotlightShader = material;
  return material;
}

function warningSelectionMaterial(item: MapLayoutItem, color: number, wireframe: boolean) {
  const material = new THREE.ShaderMaterial({
    uniforms: {
      warningTime: { value: 0 },
      warningSeed: { value: seededUnitValue(item.id) },
      warningColor: { value: new THREE.Color(color) },
      warningOpacity: { value: wireframe ? 0.96 : 0.13 },
    },
    vertexShader: `
      uniform float warningTime;
      uniform float warningSeed;
      varying float vWarningPulse;
      void main() {
        vec3 transformed = vec3(position);
        float warningSeedValue = warningSeed;
        ${warningDeformSnippet}
        vWarningPulse = warningSpike;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(transformed, 1.0);
      }
    `,
    fragmentShader: `
      uniform vec3 warningColor;
      uniform float warningOpacity;
      varying float vWarningPulse;
      void main() {
        vec3 hot = mix(warningColor, vec3(1.0, 0.92, 0.68), clamp(vWarningPulse, 0.0, 1.0) * 0.45);
        gl_FragColor = vec4(hot, warningOpacity);
      }
    `,
    wireframe,
    transparent: true,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    side: THREE.DoubleSide,
  });
  material.userData.warningShader = material;
  return material;
}

function selectionGlowFor(item: MapLayoutItem, geometry: THREE.BufferGeometry, material: THREE.Material) {
  if (item.type === "namespace" || item.type === "category") {
    const radius = item.type === "namespace" ? item.size * 1.02 : item.size * 0.98;
    const ring = new THREE.Mesh(new THREE.TorusGeometry(radius, 0.05, 6, 96), material);
    ring.rotation.x = Math.PI / 2;
    ring.position.y = item.type === "namespace" ? 0.05 : 0.16;
    return ring;
  }
  const glow = new THREE.Mesh(geometry, material);
  glow.scale.setScalar(1.08);
  return glow;
}

function selectionEffectGeometry(item: MapLayoutItem) {
  switch (item.type) {
    case "namespace":
      return new THREE.CylinderGeometry(item.size * 1.03, item.size * 0.96, 0.32, 32);
    case "category":
      return new THREE.CylinderGeometry(item.size * 0.92, item.size * 1.08, 0.46, 6);
    case "node":
      return new THREE.BoxGeometry(1.92, Math.max(3.0, item.size * 1.12), 1.92);
    case "workload":
      return new THREE.BoxGeometry(item.size * 1.22, item.size * 1.02, item.size * 1.22);
    case "service":
      return new THREE.OctahedronGeometry(item.size * 1.28, 1);
    case "warning":
      return new THREE.IcosahedronGeometry(item.size * 1.24, 1);
    case "clusterResource":
      return new THREE.DodecahedronGeometry(item.size * 1.22, 0);
    case "pod":
    default:
      return new THREE.IcosahedronGeometry(item.size * 1.22, 1);
  }
}

function colorFor(statusKey: string, type: MapItemType) {
  if (type === "category") {
    return 0x22d3ee;
  }
  if (type === "clusterResource") {
    return 0xa7f3d0;
  }
  if (type === "service") {
    return 0x67e8f9;
  }
  if (type === "namespace") {
    return 0x2dd4bf;
  }
  switch (statusKey) {
    case "danger":
      return 0xf97066;
    case "warn":
      return 0xfbbf24;
    case "good":
      return 0x34d399;
    default:
      return 0xa7f3d0;
  }
}

function setObjectVisualState(
  object: THREE.Object3D,
  state: {
    highlighted: boolean;
    scale: number;
  },
) {
  const instance = object.userData.instancedInstance as InstancedInstance | undefined;
  if (instance) {
    const nextScale = instance.baseScale.clone().multiplyScalar(state.scale);
    const nextColor = instance.baseColor.clone();
    if (state.highlighted) {
      nextColor.lerp(new THREE.Color(0xffffff), 0.34);
    }
    object.scale.copy(nextScale);
    writeInstancedInstance(instance, nextScale, nextColor, object.position);
    return;
  }
  object.traverse((child) => {
    const mesh = child as THREE.Mesh;
    const material = mesh.material as THREE.Material | THREE.Material[] | undefined;
    for (const item of Array.isArray(material) ? material : material ? [material] : []) {
      const transparentMaterial = item as THREE.Material & { opacity?: number };
      if (typeof transparentMaterial.opacity !== "number") {
        continue;
      }
      if (item.userData.baseOpacity === undefined) {
        item.userData.baseOpacity = transparentMaterial.opacity;
      }
      const baseOpacity = Number(item.userData.baseOpacity);
      if (state.highlighted) {
        transparentMaterial.opacity = 1;
      } else {
        transparentMaterial.opacity = baseOpacity;
      }
    }
  });
  object.scale.setScalar(state.scale);
}

function zoomDistanceFor(item: MapLayoutItem) {
  switch (item.type) {
    case "namespace":
    case "category":
    case "clusterResource":
      return Math.max(14, item.size * 1.8);
    case "node":
      return Math.max(12, item.size * 2.1);
    case "workload":
      return 9;
    case "service":
    case "warning":
    case "pod":
    default:
      return 7;
  }
}

function easeOutCubic(value: number) {
  return 1 - Math.pow(1 - value, 3);
}

function interactiveParent(object: THREE.Object3D): THREE.Object3D {
  let current: THREE.Object3D = object;
  while (current.parent && !current.userData.item) {
    current = current.parent;
  }
  return current;
}

function disposeObject(object: THREE.Object3D) {
  if (object.userData.skipDispose) {
    return;
  }
  object.traverse((child) => {
    const mesh = child as THREE.Mesh;
    mesh.geometry?.dispose();
    const material = mesh.material as THREE.Material | THREE.Material[] | undefined;
    if (Array.isArray(material)) {
      material.forEach((item) => {
        if (!item.userData.sharedSceneMaterial) {
          item.dispose();
        }
      });
    } else if (!material?.userData.sharedSceneMaterial) {
      material?.dispose();
    }
  });
}

document.addEventListener("DOMContentLoaded", () => {
  installMapDebugHook();
  initClusterMaps();
  const observer = new MutationObserver(() => {
    cleanupClusterMaps();
    initClusterMaps();
  });
  observer.observe(document.body, { childList: true, subtree: true });
});
